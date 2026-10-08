// Package app wires the runall CLI.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	"github.com/ludvigalden/toolkit-go/pkg/proclock"
	"github.com/ludvigalden/toolkit-go/pkg/procrun"
	"github.com/ludvigalden/toolkit-go/pkg/exitcode"

	"example.com/process_runner/pkg/go/runall"
)

// Root is the grammar: every positional argument is one command line.
type Root struct {
	Dir        string        `help:"Working directory; also the lock scope." default:"." type:"existingdir"`
	Timeout    time.Duration `help:"Per-step timeout." default:"30s"`
	Background bool          `help:"Run steps at low priority (nice)."`
	Steps      []string      `arg:"" help:"Commands to run, each one quoted string."`
}

// Main runs the tool and returns its exit code.
func Main(args []string) int {
	var root Root
	parser, err := kong.New(&root, kong.Name("runall"), kong.Description("Run commands in order, one instance per directory."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "runall:", err)
		return int(exitcode.Software)
	}
	if _, err := parser.Parse(args); err != nil {
		parser.FatalIfErrorf(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// One run per directory: a second runall in the same directory
	// fails fast and names the owner instead of interleaving.
	cache, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "runall:", err)
		return int(exitcode.Unavailable)
	}
	lock, err := proclock.Acquire(ctx, proclock.Options{
		Dir: filepath.Join(cache, "runall", "locks"), Name: "runall", Repo: root.Dir,
	})
	var held *proclock.HeldError
	if errors.As(err, &held) {
		fmt.Fprintf(os.Stderr, "runall: already running here (pid %d)\n", held.Owner.PID)
		return int(exitcode.TempFail)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "runall:", err)
		return int(exitcode.Unavailable)
	}
	defer func() { _ = lock.Release() }()

	r := runall.Runner{
		RC:      procrun.NewRunContext(root.Dir, procrun.WithScoper(runall.NiceScoper(10))),
		Timeout: root.Timeout,
		OnLine:  func(step, line string) { fmt.Printf("%s | %s\n", step, line) },
	}
	if root.Background {
		r.Profile = "background"
	}

	failed := 0
	for _, raw := range root.Steps {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		out, err := r.Run(ctx, runall.Step{Name: fields[0], Args: fields[1:]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "runall: %s: %v\n", fields[0], err)
			failed++
			continue
		}
		fmt.Fprintf(os.Stderr, "runall: %s exit=%d lines=%d time=%s peak_rss=%d timed_out=%t\n",
			out.Step, out.ExitCode, out.Lines, out.Duration.Round(time.Millisecond), out.PeakRSS, out.TimedOut)
		if out.ExitCode != 0 {
			failed++
		}
	}
	if failed > 0 {
		return int(exitcode.Failure)
	}
	return int(exitcode.OK)
}
