package serve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// DefaultKillGrace is the SIGTERM-to-SIGKILL grace period used by
// [TerminatePID] when grace is zero.
const DefaultKillGrace = 5 * time.Second

const (
	termPoll = 50 * time.Millisecond
	reapWait = time.Second
	reapPoll = 20 * time.Millisecond
)

// TerminatePID sends SIGTERM, waits up to grace, then SIGKILLs and waits
// for the reap; an already dead target is success. No identity is
// checked, so the PID must come from the caller's own live supervision
// state: otherwise it may have been reused by an unrelated process.
func TerminatePID(ctx context.Context, pid int, grace time.Duration) error {
	if grace <= 0 {
		grace = DefaultKillGrace
	}
	if err := signalPID(pid, syscall.SIGTERM); err != nil && !processGone(err) {
		return fmt.Errorf("serve: signal pid %d: %w", pid, err)
	}
	deadline := time.NewTimer(grace)
	defer deadline.Stop()
	ticker := time.NewTicker(termPoll)
	defer ticker.Stop()
	for {
		if !pidAlive(pid) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("serve: wait for pid %d: %w", pid, ctx.Err())
		case <-deadline.C:
			if err := signalPID(pid, syscall.SIGKILL); err != nil && !processGone(err) {
				return fmt.Errorf("serve: kill pid %d: %w", pid, err)
			}
			if !waitGone(ctx, pid, reapWait) {
				return fmt.Errorf("serve: pid %d did not exit after SIGKILL", pid)
			}
			return nil
		case <-ticker.C:
		}
	}
}

func processGone(err error) bool {
	return errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone)
}

func signalPID(pid int, sig syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(sig); err != nil {
		return fmt.Errorf("signal %d: %w", pid, err)
	}
	return nil
}

// pidAlive reports whether pid is a live process. EPERM means alive but
// owned by someone else; every other error means gone.
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func waitGone(ctx context.Context, pid int, budget time.Duration) bool {
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	ticker := time.NewTicker(reapPoll)
	defer ticker.Stop()
	for {
		if !pidAlive(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return !pidAlive(pid)
		case <-ticker.C:
		}
	}
}
