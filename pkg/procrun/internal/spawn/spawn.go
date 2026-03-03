// Package spawn starts one subprocess in its own process group with
// capped output capture or streamed stdout, and classifies how it ended.
package spawn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/capbuf"
	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/linestream"
	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/pgroup"
)

// streamBuffer is the line and chunk channel capacity for streaming.
const streamBuffer = 64

// Config describes one resolved invocation.
type Config struct {
	Path  string
	Args  []string
	Dir   string
	Env   []string // nil inherits the current environment
	Stdin io.Reader
	// Timeout kills the process group after this long (0: none).
	Timeout time.Duration
	// Grace sends SIGTERM this long before SIGKILL on timeout (0: SIGKILL).
	Grace time.Duration
	// OnKill observes group kills made by the timeout hook.
	OnKill func(detail string)
	// Streaming pipes stdout to Lines/Raw instead of capturing it.
	Streaming bool
	// Setup runs on the armed command just before it starts.
	Setup func(cmd *exec.Cmd) error
}

// Command builds the exec.Cmd for cfg without arming or starting it.
func Command(ctx context.Context, cfg Config) *exec.Cmd {
	cmd := exec.CommandContext(ctx, cfg.Path, cfg.Args...)
	cmd.Dir = cfg.Dir
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	}
	if cfg.Stdin != nil {
		cmd.Stdin = cfg.Stdin
	}
	return cmd
}

// Proc is a started subprocess. Wait must be called exactly once.
type Proc struct {
	Cmd    *exec.Cmd
	Lines  chan string // streaming only
	Raw    chan []byte // streaming only
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	stdout *capbuf.Buffer
	stderr *capbuf.Buffer
	reaped func()
}

// Start arms, sets up, and starts cfg. Errors from Setup are returned as
// is; start failures are wrapped with display.
func Start(ctx context.Context, cfg Config, display string) (*Proc, error) {
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}
	fail := func(err error) (*Proc, error) {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	onKill := cfg.OnKill
	if onKill == nil {
		onKill = func(string) {}
	}
	cmd := Command(ctx, cfg)
	pgroupDone := pgroup.Arm(cmd, cfg.Grace, onKill)
	p := &Proc{Cmd: cmd, ctx: ctx, cancel: cancel, stderr: &capbuf.Buffer{}, reaped: pgroupDone}
	cmd.Stderr = p.stderr
	if cfg.Setup != nil {
		if err := cfg.Setup(cmd); err != nil {
			return fail(err)
		}
	}
	var pipe io.ReadCloser
	if cfg.Streaming {
		var err error
		if pipe, err = cmd.StdoutPipe(); err != nil {
			return fail(fmt.Errorf("procrun: stdout pipe for %s: %w", display, err))
		}
		p.Lines = make(chan string, streamBuffer)
		p.Raw = make(chan []byte, streamBuffer)
		p.done = make(chan struct{})
	} else {
		p.stdout = &capbuf.Buffer{}
		cmd.Stdout = p.stdout
	}
	if err := cmd.Start(); err != nil {
		if cfg.Streaming {
			close(p.Lines)
			close(p.Raw)
		}
		return fail(fmt.Errorf("procrun: start %s: %w", display, err))
	}
	if cfg.Streaming {
		go linestream.Pump(ctx, pipe, p.Lines, p.Raw, p.done)
	}
	return p, nil
}

// Status classifies how a process ended.
type Status int

// Outcomes in Wait's precedence order: exit 0, timeout fired, caller's
// ctx ended, I/O drain grace expired, non-zero exit (ExitCode set), and
// any other wait failure.
const (
	OK Status = iota
	Deadline
	Canceled
	IODrain
	Exited
	Failed
)

// Outcome is the collected result of Wait. ExitCode is -1 unless the
// process ran to an exit status.
type Outcome struct {
	Status          Status
	ExitCode        int
	Err             error // the exec error; *exec.ExitError for Exited
	Stdout, Stderr  []byte
	StdoutTruncated bool
	StderrTruncated bool
}

// Wait reaps the process, drains streaming output, and classifies the
// outcome. after runs once the process is reaped, before the drain.
func (p *Proc) Wait(after func()) Outcome {
	execErr := p.Cmd.Wait()
	if p.reaped != nil {
		p.reaped()
	}
	if after != nil {
		after()
	}
	if p.cancel != nil {
		// Deferred: ctx.Err must be read before cancel masks a deadline.
		defer p.cancel()
	}
	if p.done != nil {
		<-p.done
	}
	o := Outcome{Err: execErr, Stderr: p.stderr.Bytes(), StderrTruncated: p.stderr.Truncated()}
	if p.stdout != nil {
		o.Stdout, o.StdoutTruncated = p.stdout.Bytes(), p.stdout.Truncated()
	}
	if execErr == nil {
		return o
	}
	o.ExitCode = -1
	var ee *exec.ExitError
	switch {
	case errors.Is(p.ctx.Err(), context.DeadlineExceeded):
		o.Status = Deadline
	case p.ctx.Err() != nil:
		o.Status = Canceled
	case errors.Is(execErr, exec.ErrWaitDelay):
		o.Status = IODrain
	case errors.As(execErr, &ee):
		o.Status, o.ExitCode = Exited, ee.ExitCode()
	default:
		o.Status = Failed
	}
	return o
}
