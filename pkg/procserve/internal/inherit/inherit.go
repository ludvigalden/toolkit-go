// Package inherit hands a pre-bound listener and the identity lock to a
// detached child as fds 3 and 4, and adopts them on the child side.
package inherit

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"syscall"

	"golang.org/x/sys/unix"
)

// envMarker marks a process spawned by Spawn.
const envMarker = "PROCSERVE_INHERITED"

// Inherited descriptor numbers (ExtraFiles start at 3).
const (
	fdListener = 3
	fdLock     = 4
)

// Spawned reports whether this process was started by Spawn.
func Spawned(getenv func(string) string) bool { return getenv(envMarker) == "1" }

// Spawn starts cmd detached (setsid) with ln as fd 3 and lock as fd 4,
// and reaps it in the background.
func Spawn(cmd *exec.Cmd, ln *net.UnixListener, lock *os.File) error {
	if len(cmd.ExtraFiles) > 0 {
		return errors.New("procserve: Command must not set ExtraFiles")
	}
	lnFile, err := ln.File()
	if err != nil {
		return fmt.Errorf("procserve: listener file: %w", err)
	}
	defer lnFile.Close() //nolint:errcheck // parent copy
	cmd.ExtraFiles = []*os.File{lnFile, lock}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(slices.Clip(env), envMarker+"=1")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("procserve: spawn: %w", err)
	}
	go cmd.Wait() //nolint:errcheck // reaped only to avoid a zombie
	return nil
}

// Adopt takes over the inherited lock and listener. It clears the spawn
// marker so this process's own children are not mistaken for servers.
func Adopt() (*net.UnixListener, *os.File, error) {
	_ = os.Unsetenv(envMarker) //nolint:errcheck // best-effort
	unix.CloseOnExec(fdListener)
	unix.CloseOnExec(fdLock)
	lock := os.NewFile(fdLock, "procserve-lock")
	f := os.NewFile(fdListener, "procserve-listener")
	ln, err := net.FileListener(f)
	_ = f.Close() //nolint:errcheck // FileListener holds a dup
	if err != nil {
		_ = lock.Close() //nolint:errcheck // adoption failed
		return nil, nil, fmt.Errorf("procserve: inherited listener: %w", err)
	}
	ul, ok := ln.(*net.UnixListener)
	if !ok {
		_ = ln.Close()   //nolint:errcheck // wrong kind
		_ = lock.Close() //nolint:errcheck // adoption failed
		return nil, nil, errors.New("procserve: inherited listener is not a unix socket")
	}
	ul.SetUnlinkOnClose(false)
	return ul, lock, nil
}
