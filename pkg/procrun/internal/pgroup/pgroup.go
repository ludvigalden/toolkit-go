// Package pgroup runs a command in its own process group and arms its
// cancel hook to signal the whole group, so a timeout reaches every
// descendant and not only the direct child.
package pgroup

import (
	"fmt"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// DrainGrace bounds how long Wait drains I/O pipes after the group was
// killed: an orphan holding stderr must not block Wait forever.
const DrainGrace = 2 * time.Second

// Arm gives cmd its own process group and a cancel hook: SIGTERM, then
// SIGKILL after grace (> 0), reporting each SIGKILL to onKill. Call the
// returned func once cmd.Wait has returned: it skips any pending
// SIGKILL so the signal cannot land on a recycled PID.
func Arm(cmd *exec.Cmd, grace time.Duration, onKill func(detail string)) func() {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if grace <= 0 {
		cmd.WaitDelay = DrainGrace
		cmd.Cancel = func() error {
			onKill("timeout")
			return Signal(cmd.Process.Pid, syscall.SIGKILL)
		}
		return func() {}
	}
	// WaitDelay must exceed the grace, or the stdlib's own kill would cap it.
	cmd.WaitDelay = grace + DrainGrace
	var reaped atomic.Bool
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		err := Signal(pid, syscall.SIGTERM)
		time.AfterFunc(grace, func() {
			if reaped.Load() {
				return
			}
			if Signal(pid, syscall.SIGKILL) == nil {
				onKill("timeout after grace")
			}
		})
		return err
	}
	return func() { reaped.Store(true) }
}

// Signal delivers sig to the process group led by pid.
func Signal(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err != nil {
		return fmt.Errorf("procrun: signal group %d: %w", pid, err)
	}
	return nil
}
