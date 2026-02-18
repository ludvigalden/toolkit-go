// Package cli turns a Kong grammar into a configured invocation: it
// parses arguments, layers configuration (defaults, files, dotenv,
// environment, flags), and wires output and logging, so a tool's main
// is a few lines.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"

	"github.com/ludvigalden/toolkit-go/pkg/cli/internal/kongx"
	"github.com/ludvigalden/toolkit-go/pkg/cli/internal/wiring"
	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
	"github.com/ludvigalden/toolkit-go/pkg/output"
)

// App describes a tool. Only Name is required; zero fields fall back to
// the process (os.Stdout, os.Stderr, os.Getenv).
type App struct {
	Name        string
	Description string
	// EnvPrefix prefixes the tool's environment variables; it defaults
	// to Name upper-cased with dashes as underscores.
	EnvPrefix string
	Version   string
	Stdout    io.Writer
	Stderr    io.Writer
	// Defaults are the lowest-priority config values, keyed like the
	// config files ("<name>.port"). A key must be known (here or in a
	// file) for its environment variable to bind.
	Defaults map[string]any
	// LocalConfigNames are the project files searched from the working
	// directory up to $HOME; nil means "<name>.yaml" and ".<name>.yaml".
	LocalConfigNames []string
	Getenv           func(string) string
	// OutputOptions extend output.New, e.g. a custom renderer.
	OutputOptions []output.Option
}

// Main parses args, runs the selected command with binds, and returns
// the exit code. It is Parse followed by Run.Exec.
func (a *App) Main(grammar any, args []string, binds ...any) int {
	run, code := a.Parse(grammar, args)
	if run == nil {
		return code
	}
	return run.Exec(binds...)
}

// Parse parses args into grammar and prepares the invocation. A nil Run
// means exit with the returned code: 0 after --help, Usage for bad
// arguments or a bad command-line --stdout-format, Config or CantCreate
// when config or output wiring fails for other reasons.
func (a *App) Parse(grammar any, args []string) (*Run, int) {
	stderr := a.stderr()
	parser, err := kong.New(grammar,
		kong.Name(a.Name),
		kong.Description(a.Description),
		kong.Writers(stderr, stderr),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
		kong.Vars{"version": a.Version},
		kong.Exit(kongx.Exit),
		kong.UsageOnError(),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: invalid grammar: %v\n", a.Name, err)
		return nil, int(exitcode.Software)
	}
	kctx, code, done := kongx.Parse(parser, args, int(exitcode.Usage))
	if done {
		return nil, code
	}
	if kctx.Empty() {
		_ = kctx.PrintUsage(false) //nolint:errcheck // usage on stderr is best effort
		return nil, int(exitcode.OK)
	}
	run, err := a.prepare(grammar, kctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", a.Name, err)
		var ce codedError
		if errors.As(err, &ce) {
			return nil, int(ce.code)
		}
		return nil, int(exitcode.Config)
	}
	return run, int(exitcode.OK)
}

func (a *App) stdout() io.Writer {
	if a.Stdout != nil {
		return a.Stdout
	}
	return os.Stdout
}

func (a *App) stderr() io.Writer {
	if a.Stderr != nil {
		return a.Stderr
	}
	return os.Stderr
}

func (a *App) getenv(key string) string {
	if a.Getenv != nil {
		return a.Getenv(key)
	}
	return os.Getenv(key)
}

func (a *App) envPrefix() string { return wiring.EnvPrefix(a.Name, a.EnvPrefix) }

// codedError carries a setup failure's exit code to Parse.
type codedError struct {
	code exitcode.Code
	err  error
}

func (e codedError) Error() string { return e.err.Error() }
func (e codedError) Unwrap() error { return e.err }
