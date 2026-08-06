// Package prockill puts a child command in its own process group and
// kills the whole group on context cancellation, so cancellation never
// orphans grandchildren; WaitDelay bounds the post-kill wait.
package prockill

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// waitDelay bounds how long Wait keeps waiting for I/O pipes to close
// after the process group was killed.
const waitDelay = 2 * time.Second

// Configure gives cmd its own process group and arms a group-wide
// SIGKILL as the context-cancel hook.
func Configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return fmt.Errorf("kill scratch process group %d: %w", cmd.Process.Pid, err)
		}
		return nil
	}
	cmd.WaitDelay = waitDelay
}
