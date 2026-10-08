// Package runall runs a list of commands one after another, streaming
// each command's output and sampling its resource use.
package runall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procrun"
)

// Step is one command to run.
type Step struct {
	Name string
	Args []string
}

// Outcome is what one step produced.
type Outcome struct {
	Step     string        `json:"step"`
	ExitCode int           `json:"exit_code"`
	Lines    int           `json:"lines"`
	Duration time.Duration `json:"duration_ns"`
	PeakRSS  uint64        `json:"peak_rss_bytes"`
	TimedOut bool          `json:"timed_out"`
}

// NiceScoper lowers the priority of every Spec that names the
// "background" profile by running it under nice(1). It shows the
// procrun.Scoper seam: the scoper rewrites cmd before it starts and
// returns the function that undoes its governance (nothing, here).
func NiceScoper(level int) procrun.Scoper {
	return procrun.ScoperFunc(func(_ context.Context, profile string, cmd *exec.Cmd) (func(context.Context) error, error) {
		if profile != "background" {
			return nil, nil // nothing to undo
		}
		nice, err := exec.LookPath("nice")
		if err != nil {
			return nil, fmt.Errorf("background profile needs nice: %w", err)
		}
		cmd.Args = append([]string{nice, "-n", fmt.Sprint(level)}, cmd.Args...)
		cmd.Path = nice
		return nil, nil
	})
}

// Runner runs steps in one working directory.
type Runner struct {
	RC      *procrun.RunContext
	Timeout time.Duration
	// Profile, when set, names the procrun profile every step uses.
	Profile string
	// OnLine receives each stdout line as it arrives.
	OnLine func(step, line string)
}

// Run executes one step, streaming its stdout to OnLine.
func (r Runner) Run(ctx context.Context, s Step) (Outcome, error) {
	spec := r.RC.Cmd(s.Name, s.Args...)
	if r.Timeout > 0 {
		spec = spec.Timeout(r.Timeout)
	}
	if r.Profile != "" {
		spec = spec.Profile(r.Profile)
	}
	start := time.Now()
	proc, err := spec.Start(ctx)
	if err != nil {
		return Outcome{Step: s.Name, ExitCode: -1}, err
	}
	out := Outcome{Step: s.Name}
	done := make(chan struct{})
	peak := make(chan uint64, 1)
	go func() {
		// Sample RSS while the step runs; a short step may finish first.
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		var max uint64
		for {
			if st, err := procrun.ReadStats(proc.Pid()); err == nil && st.RSSBytes > max {
				max = st.RSSBytes
			}
			select {
			case <-done:
				peak <- max
				return
			case <-t.C:
			}
		}
	}()
	err = procrun.StreamLines(proc, func(ev procrun.Event) bool {
		if ev.Kind == procrun.KindLine {
			out.Lines++
			if r.OnLine != nil {
				r.OnLine(s.Name, ev.Raw)
			}
		}
		return true
	})
	close(done)
	out.PeakRSS = <-peak
	out.Duration = time.Since(start)
	var exitErr *procrun.ExitError
	var timeoutErr *procrun.TimeoutError
	switch {
	case errors.As(err, &timeoutErr):
		out.TimedOut, out.ExitCode = true, -1
		return out, nil
	case errors.As(err, &exitErr):
		out.ExitCode = exitErr.ExitCode
		return out, nil
	}
	return out, err
}
