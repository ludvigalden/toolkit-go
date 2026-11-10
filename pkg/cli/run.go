package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/knadh/koanf/v2"

	"github.com/ludvigalden/toolkit-go/pkg/cli/internal/cascade"
	"github.com/ludvigalden/toolkit-go/pkg/cli/internal/kongx"
	"github.com/ludvigalden/toolkit-go/pkg/cli/internal/wiring"
	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
	"github.com/ludvigalden/toolkit-go/pkg/output"
	"github.com/ludvigalden/toolkit-go/pkg/xdg"
)

// Run is one prepared invocation. Exec runs the selected command with
// Out, Logger, Config and Global bound, so a command's Run method can
// take any of them as a parameter.
type Run struct {
	Kong   *kong.Context
	Config *koanf.Koanf
	Global Global
	Out    *output.Output
	Logger *slog.Logger
	sink   io.Closer
}

// Paths are the tool's XDG directories. None is created; make the ones
// you write to.
type Paths struct {
	Config string
	Cache  string
	State  string
	Data   string
}

// Exec runs the selected command, reports a returned error through
// Out, closes the run, and returns the exit code: errkind.ExitCode of
// the error, so classified errors choose their own sysexits code.
func (r *Run) Exec(binds ...any) int {
	binds = append([]any{r.Out, r.Logger, r.Config, &r.Global}, binds...)
	err := r.Kong.Run(binds...)
	if err != nil {
		r.Out.Fail(err)
	}
	if cerr := r.Close(); cerr != nil && err == nil {
		_, _ = fmt.Fprintf(r.Kong.Stderr, "%s: closing output: %v\n", r.Global.AppName, cerr)
		return int(exitcode.Software)
	}
	return int(errkind.ExitCode(err))
}

// Close finishes the output document and closes the debug log.
func (r *Run) Close() error {
	err := r.Out.Close()
	if r.sink != nil {
		_ = r.sink.Close() //nolint:errcheck // the log file is best effort
		r.sink = nil
	}
	return err
}

// globalsCarrier finds the embedded GlobalFlags in a grammar.
type globalsCarrier interface{ cliGlobals() *GlobalFlags }

func (a *App) prepare(grammar any, kctx *kong.Context) (*Run, error) {
	getenv := a.getenv
	var g GlobalFlags
	if c, ok := grammar.(globalsCarrier); ok {
		g = *c.cliGlobals()
	}
	cwd, err := cascade.PhysicalCwd()
	if err != nil {
		return nil, err
	}
	home, err := cascade.HomeDir(getenv)
	if err != nil {
		return nil, err
	}
	dirs, err := xdg.Resolve(getenv)
	if err != nil {
		return nil, err
	}
	prefix := a.envPrefix()
	profile := cascade.Profile(g.Profile, prefix, getenv)
	l := cascade.Layers{
		Name: a.Name, Prefix: prefix, Defaults: cascade.WithBuiltins(a.Defaults),
		UserDir: filepath.Join(dirs.ConfigHome, a.Name), Profile: profile,
		Walk: cascade.WalkUp(cwd, home), LocalNames: cascade.LocalNamesFor(a.Name, a.LocalConfigNames),
		Files: g.Config, EnvFiles: g.EnvFile,
		// The App.Getenv convention encodes "unset" as "", so only a
		// non-empty value carries presence here.
		Getenv: func(key string) (string, bool) {
			v := getenv(key)
			return v, v != ""
		},
		Flags: kongx.FlagOverrides(kctx),
	}
	k, err := l.Load()
	if err != nil {
		return nil, err
	}
	glob, err := a.global(k, cwd, profile, dirs)
	if err != nil {
		return nil, codedError{exitcode.Usage, err}
	}
	run := &Run{Kong: kctx, Config: k, Global: glob}
	if err := a.wire(run, k); err != nil {
		code := exitcode.CantCreate
		var me outputModeError
		if errors.As(err, &me) {
			if _, explicit := kongx.FlagOverrides(kctx)["stdout_format"]; explicit {
				code = exitcode.Usage
			}
		}
		return nil, codedError{code, err}
	}
	return run, nil
}

func (a *App) global(k *koanf.Koanf, cwd, profile string, dirs xdg.Dirs) (Global, error) {
	// Any name is accepted here; output.New rejects a mode that has
	// neither a built-in nor a registered renderer.
	mode := output.Mode(strings.TrimSpace(k.String("stdout_format")))
	if mode == "" {
		mode = output.ModeText
	}
	verb, err := output.ParseVerbosity(k.String("stdout_verbosity"))
	if err != nil {
		return Global{}, err
	}
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	session := k.String("session")
	if session == "" {
		session = wiring.SessionID()
	}
	return Global{
		AppName: a.Name, Profile: profile, Cwd: cwd, ExeDir: exeDir,
		SessionID: session, OutputMode: mode, Verbosity: verb,
		LogLevel: verb.Level(), Debug: k.Bool("debug") || verb >= output.VerbosityDebug,
		Dirs: Paths{
			Config: filepath.Join(dirs.ConfigHome, a.Name),
			Cache:  filepath.Join(dirs.CacheHome, a.Name),
			State:  filepath.Join(dirs.StateHome, a.Name),
			Data:   filepath.Join(dirs.DataHome, a.Name),
		},
	}, nil
}

// wire builds Out and Logger. With a debug_log path every diagnostic
// and debug-level log record is also appended there as JSON lines.
func (a *App) wire(run *Run, k *koanf.Koanf) error {
	color, err := output.ParseColorMode(k.String("color"))
	if err != nil {
		return err
	}
	stderr := a.stderr()
	opts := []output.Option{
		output.WithMode(run.Global.OutputMode),
		output.WithVerbosity(run.Global.Verbosity),
		output.WithColor(color.Enabled(stderr, a.getenv)),
	}
	level := run.Global.LogLevel
	if path := k.String("debug_log"); path != "" {
		f, h, err := wiring.DebugLog(path)
		if err != nil {
			return err //nolint:wrapcheck // DebugLog names the failure
		}
		run.sink = f
		opts = append(opts, output.WithLogHandler(h))
		level = min(level, slog.LevelDebug)
	}
	out, err := output.New(a.stdout(), stderr, append(opts, a.OutputOptions...)...)
	if err != nil {
		if run.sink != nil {
			_ = run.sink.Close() //nolint:errcheck // already failing
		}
		return outputModeError{err}
	}
	run.Out = out
	run.Logger = slog.New(output.NewSlogHandler(out, level))
	return nil
}

// outputModeError marks a renderer-creation failure: when the stdout
// format came from the command line it is a usage error, otherwise
// (config, env, a failing factory) the output could not be created.
type outputModeError struct{ err error }

func (e outputModeError) Error() string { return e.err.Error() }
func (e outputModeError) Unwrap() error { return e.err }
