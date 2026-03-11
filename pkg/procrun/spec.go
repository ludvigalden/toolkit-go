package procrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/pgroup"
	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/shell"
	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/spawn"
	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/track"
)

// Spec describes one subprocess invocation. Build it with Cmd/CmdPath and
// chain setters; Run or Start executes it.
type Spec struct {
	name          string
	args          []string
	path          string
	dir           string
	env           []string
	timeout       time.Duration
	gracefulStop  time.Duration
	shellFallback bool
	shellExec     bool
	longRunning   bool
	stdin         io.Reader
	rc            *RunContext
	profile       string
}

// Dir sets the working directory.
func (s *Spec) Dir(dir string) *Spec { s.dir = dir; return s }

// Env sets the full environment; nil inherits the current process's.
func (s *Spec) Env(env []string) *Spec { s.env = env; return s }

// Stdin sets the subprocess standard input (default: closed).
func (s *Spec) Stdin(r io.Reader) *Spec { s.stdin = r; return s }

// Timeout kills the process group after d and returns a *TimeoutError.
func (s *Spec) Timeout(d time.Duration) *Spec { s.timeout = d; return s }

// GracefulStop makes timeout expiry send SIGTERM to the group first and
// SIGKILL d later. Non-positive values keep the SIGKILL-first stop.
func (s *Spec) GracefulStop(d time.Duration) *Spec {
	if d > 0 {
		s.gracefulStop = d
	}
	return s
}

// ShellFallback re-runs the command through a non-interactive $SHELL -c
// when the binary is not resolvable, in its own process group and with
// the context's shell-child marker exported.
func (s *Spec) ShellFallback() *Spec { s.shellFallback = true; return s }

// LongRunning tags the process for graceful SIGTERM-then-kill in
// Registry.Cleanup.
func (s *Spec) LongRunning() *Spec { s.longRunning = true; return s }

// Profile names the resource profile handed to the context's Scoper; an
// empty profile skips the Scoper.
func (s *Spec) Profile(name string) *Spec { s.profile = name; return s }

// String renders the command line for messages.
func (s *Spec) String() string {
	if len(s.args) == 0 {
		return s.name
	}
	return s.name + " " + strings.Join(s.args, " ")
}

// Run executes the command and waits, capturing stdout and stderr (each
// capped at 16 MiB, head-kept).
func (s *Spec) Run(ctx context.Context) (*Result, error) {
	start := time.Now()
	proc, err := s.start(ctx, false)
	if err != nil {
		return nil, err
	}
	res, werr := proc.Wait()
	s.rc.observe(DiagEvent{Kind: DiagExit, Name: s.name, Elapsed: time.Since(start)})
	return res, werr
}

// Start executes the command for streaming; the caller must call Wait.
func (s *Spec) Start(ctx context.Context) (*Process, error) {
	return s.start(ctx, true)
}

// Prepare resolves and scopes the command and returns it unstarted; the
// caller owns stdio, Start, and Release.
func (s *Spec) Prepare(ctx context.Context) (*PreparedCmd, error) {
	target, err := s.resolveTarget()
	if err != nil {
		return nil, err
	}
	cmd := spawn.Command(ctx, target.config())
	var groupDone func()
	if target.shellExec {
		groupDone = target.rc.applyGroupKill(cmd, target.name, DiagShellKill, target.gracefulStop)
	}
	prepared := &PreparedCmd{cmd: cmd, groupDone: groupDone}
	release, err := target.scope(ctx, cmd)
	if err != nil {
		return nil, err
	}
	prepared.release = release
	return prepared, nil
}

// resolveTarget returns the Spec to execute: s with its binary resolved,
// or the $SHELL -c wrapper when resolution failed and ShellFallback is set.
func (s *Spec) resolveTarget() (*Spec, error) {
	if s.path != "" {
		return s, nil
	}
	bin, err := s.rc.FindWithoutShell(s.name)
	if err == nil {
		cp := *s
		cp.path = bin
		return &cp, nil
	}
	sh := s.rc.envLookup("SHELL")
	if !s.shellFallback || sh == "" {
		return nil, err
	}
	cp := *s
	cp.path = sh
	cp.args = []string{"-c", shell.Line(s.name, s.args)}
	cp.shellExec = true
	return &cp, nil
}

// config builds the spawn configuration for a resolved Spec, exporting
// the shell-child marker into a shell fallback's environment.
func (s *Spec) config() spawn.Config {
	cfg := spawn.Config{Path: s.path, Args: s.args, Dir: s.dir, Env: s.env, Stdin: s.stdin, Timeout: s.timeout, Grace: s.gracefulStop}
	if s.shellExec {
		if cfg.Env == nil {
			cfg.Env = s.rc.spawnEnvOrProcess()
		}
		cfg.Env = append(slices.Clip(cfg.Env), s.rc.shellChildEnv+"=1")
		s.rc.emit(DiagShellSpawn, s.name, "exec: "+s.path)
	}
	return cfg
}

// scope runs the context's Scoper when the Spec names a profile.
func (s *Spec) scope(ctx context.Context, cmd *exec.Cmd) (func(context.Context) error, error) {
	if s.rc.scoper == nil || s.profile == "" {
		return nil, nil
	}
	release, err := s.rc.scoper.Wrap(ctx, s.profile, cmd)
	if err != nil {
		return nil, fmt.Errorf("procrun: scope %s: %w", s.name, err)
	}
	return release, nil
}

// start resolves, scopes, and starts the process.
func (s *Spec) start(ctx context.Context, streaming bool) (*Process, error) {
	target, err := s.resolveTarget()
	if err != nil {
		return nil, err
	}
	killKind := DiagGroupKill
	if target.shellExec {
		killKind = DiagShellKill
	}
	cfg := target.config()
	cfg.Streaming = streaming
	cfg.OnKill = func(detail string) { target.rc.emit(killKind, target.name, detail) }
	proc := &Process{name: target.name, gracefulStop: target.gracefulStop, rc: target.rc}
	cfg.Setup = func(cmd *exec.Cmd) (err error) {
		proc.release, err = target.scope(ctx, cmd)
		return err
	}
	if proc.p, err = spawn.Start(ctx, cfg, target.String()); err != nil {
		return nil, err //nolint:wrapcheck // spawn and scope errors carry the procrun prefix
	}
	proc.pid = proc.p.Cmd.Process.Pid
	target.rc.emit(DiagLaunch, target.name, "")
	kind := track.OneShot
	if target.longRunning {
		kind = track.LongRunning
	}
	target.rc.registry.register(proc, kind)
	return proc, nil
}

// applyGroupKill puts cmd in its own process group whose timeout kill
// reaches every descendant; with grace > 0 SIGTERM precedes SIGKILL.
// The returned func marks the command reaped (see pgroup.Arm).
func (c *RunContext) applyGroupKill(cmd *exec.Cmd, name string, kind DiagKind, grace time.Duration) func() {
	return pgroup.Arm(cmd, grace, func(detail string) { c.emit(kind, name, detail) })
}

// observe forwards an event to the observer, if any.
func (c *RunContext) observe(e DiagEvent) {
	if c.observer != nil {
		c.observer(e)
	}
}

// Result holds the outcome of a subprocess. The Truncated flags report
// that output hit the 16 MiB capture cap.
type Result struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
}

// Process is a started subprocess. Call Wait exactly once.
type Process struct {
	p            *spawn.Proc
	name         string
	pid          int
	gracefulStop time.Duration
	rc           *RunContext
	release      func(context.Context) error
	waited       bool
}

// Pid returns the operating system process ID.
func (p *Process) Pid() int { return p.pid }

// Stdout returns a line reader over streamed stdout (Start only). Lines
// are dropped, not blocked on, when the consumer falls 64 lines behind.
func (p *Process) Stdout() *LineReader { return &LineReader{ch: p.p.Lines} }

// Raw returns a chunk reader over streamed stdout (Start only).
func (p *Process) Raw() *RawReader { return &RawReader{ch: p.p.Raw} }

// Signal sends sig to the process.
func (p *Process) Signal(sig os.Signal) error {
	if err := p.p.Cmd.Process.Signal(sig); err != nil {
		return fmt.Errorf("procrun: signal %s: %w", p.name, err)
	}
	return nil
}

// Kill terminates the process immediately.
func (p *Process) Kill() error {
	if err := p.p.Cmd.Process.Kill(); err != nil {
		return fmt.Errorf("procrun: kill %s: %w", p.name, err)
	}
	return nil
}

// Wait reaps the process and classifies its outcome: *ExitError for a
// non-zero exit, *TimeoutError for a timeout or an I/O drain overrun,
// and a context.Canceled-wrapped error when the caller's context ended.
func (p *Process) Wait() (*Result, error) {
	if p.waited {
		return nil, &DoubleWaitError{Name: p.name}
	}
	p.waited = true
	o := p.p.Wait(func() {
		if p.release != nil {
			_ = p.release(context.Background()) //nolint:errcheck // best-effort; the run's ctx may be done
		}
		p.rc.registry.deregister(p)
	})
	res := &Result{ExitCode: o.ExitCode, Stdout: o.Stdout, Stderr: o.Stderr, StdoutTruncated: o.StdoutTruncated, StderrTruncated: o.StderrTruncated}
	switch o.Status {
	case spawn.OK:
		return res, nil
	case spawn.Deadline:
		p.rc.emit(DiagTimeout, p.name, "deadline exceeded")
		return res, &TimeoutError{Name: p.name, Timeout: "context deadline exceeded", Grace: p.gracefulStop, Err: o.Err}
	case spawn.IODrain:
		p.rc.emit(DiagTimeout, p.name, "io wait grace expired")
		return res, &TimeoutError{Name: p.name, Timeout: "I/O completion grace", Grace: p.gracefulStop, Err: o.Err}
	case spawn.Exited:
		return res, &ExitError{Name: p.name, ExitCode: o.ExitCode, Stderr: o.Stderr, Err: o.Err}
	case spawn.Canceled:
		return res, fmt.Errorf("procrun: wait %s: %w", p.name, context.Canceled)
	}
	return res, fmt.Errorf("procrun: wait %s: %w", p.name, o.Err)
}

// PreparedCmd is an unstarted exec.Cmd with any scope applied; the caller
// owns stdio and Start and calls Release after exit.
type PreparedCmd struct {
	cmd       *exec.Cmd
	release   func(context.Context) error
	groupDone func()
}

// Cmd returns the prepared command.
func (p *PreparedCmd) Cmd() *exec.Cmd { return p.cmd }

// Release undoes the scope and marks the process group reaped; a no-op
// when unscoped.
func (p *PreparedCmd) Release(ctx context.Context) error {
	if p.groupDone != nil {
		p.groupDone()
	}
	if p.release == nil {
		return nil
	}
	return p.release(ctx)
}
