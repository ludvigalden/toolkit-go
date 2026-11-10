// Package setsid is the manager-free supervisor fallback: it spawns the
// service in its own session and tracks it through the proclock the
// service itself holds. It needs no root, daemon or service manager.
package setsid

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/proclock"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

// Mode is the Status.Mode this backend reports.
const Mode = "setsid"

// Defaults; see the matching options.
const (
	DefaultStartTimeout = 15 * time.Second
	DefaultKillGrace    = 5 * time.Second
	pollInterval        = 50 * time.Millisecond
	sigkillReapBudget   = time.Second
)

// Setsid spawns services detached and finds them by their proclock. The
// service must acquire proclock.Options{Dir: lockDir, Name: Spec.UnitName}
// (or what WithLockOptions returns) and publish an endpoint once it is
// ready. Construct it with New.
type Setsid struct {
	lockOptions  func(supervisor.Spec) proclock.Options
	startTimeout time.Duration
	killGrace    time.Duration

	mu        sync.Mutex
	installed map[string]bool
}

var _ supervisor.Supervisor = (*Setsid)(nil)

// Option configures a Setsid.
type Option func(*Setsid)

// WithStartTimeout bounds how long Start waits for the child to publish
// its endpoint (0: DefaultStartTimeout).
func WithStartTimeout(d time.Duration) Option { return func(s *Setsid) { s.startTimeout = d } }

// WithKillGrace is how long Stop waits after SIGTERM before SIGKILL
// (0: DefaultKillGrace).
func WithKillGrace(d time.Duration) Option { return func(s *Setsid) { s.killGrace = d } }

// WithLockOptions locates each service's lock, for services that lock
// somewhere other than <lockDir>/global/<UnitName>.lock. It replaces the
// lockDir argument of New, which must then be "".
func WithLockOptions(f func(supervisor.Spec) proclock.Options) Option {
	return func(s *Setsid) { s.lockOptions = f }
}

// New returns a Setsid reading service locks under lockDir, the same
// proclock root the services acquire in, or where WithLockOptions says.
func New(lockDir string, opts ...Option) (*Setsid, error) {
	s := &Setsid{installed: map[string]bool{}}
	for _, o := range opts {
		o(s)
	}
	switch {
	case lockDir != "" && s.lockOptions != nil:
		return nil, errors.New("supervisor: setsid: lock dir and WithLockOptions are mutually exclusive")
	case lockDir == "" && s.lockOptions == nil:
		return nil, errors.New("supervisor: setsid: lock dir or WithLockOptions is required")
	case lockDir != "":
		s.lockOptions = func(spec supervisor.Spec) proclock.Options {
			return proclock.Options{Dir: lockDir, Name: spec.UnitName}
		}
	}
	if s.startTimeout <= 0 {
		s.startTimeout = DefaultStartTimeout
	}
	if s.killGrace <= 0 {
		s.killGrace = DefaultKillGrace
	}
	return s, nil
}

// Install records intent; there is no unit file.
func (s *Setsid) Install(_ context.Context, spec supervisor.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.installed[spec.UnitName] = true
	return nil
}

// Uninstall forgets the recorded intent.
func (s *Setsid) Uninstall(_ context.Context, spec supervisor.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.installed, spec.UnitName)
	return nil
}

// Start spawns the child once (no respawn loop) and waits until it holds
// the lock with a published endpoint; a live owner makes it a no-op. The
// instance mutex spans spawn through readiness to prevent a double spawn,
// so other calls on this Setsid wait behind a slow or hung child.
func (s *Setsid) Start(ctx context.Context, spec supervisor.Spec) error {
	if spec.ExePath == "" || spec.UnitName == "" {
		return errors.New("supervisor: setsid start: spec ExePath and UnitName are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, prevFound, readErr := s.read(spec)
	if readErr == nil && prevFound && pidAlive(prev.PID) {
		return nil
	}
	cmd, closeOut, err := command(spec)
	if err != nil {
		return err
	}
	defer closeOut()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("supervisor: setsid start: %w", err)
	}
	// Reap in the background so an early crash is collected instead of
	// lingering as a zombie that still answers signal 0.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return s.waitReady(ctx, spec, cmd.Process.Pid, prev, prevFound, exited)
}

// command builds the detached child; Setsid makes it a session leader
// that outlives the caller.
func command(spec supervisor.Spec) (*exec.Cmd, func(), error) {
	cmd := exec.Command(spec.ExePath, spec.Args...) //nolint:gosec,noctx // the child must outlive ctx
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Dir = spec.WorkingDirectory
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, nil, fmt.Errorf("supervisor: setsid start: %w", err)
	}
	cmd.Stdin = devNull
	out := devNull
	if spec.StderrLogPath != "" {
		out, err = os.OpenFile(spec.StderrLogPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			_ = devNull.Close() //nolint:errcheck // already failing
			return nil, nil, fmt.Errorf("supervisor: setsid start: log: %w", err)
		}
	}
	cmd.Stdout, cmd.Stderr = out, out
	return cmd, func() {
		_ = devNull.Close() //nolint:errcheck // child holds its own copy
		_ = out.Close()     //nolint:errcheck // child holds its own copy
	}, nil
}

func (s *Setsid) waitReady(ctx context.Context, spec supervisor.Spec, pid int, prev proclock.Owner, prevFound bool, exited <-chan error) error {
	deadline := time.NewTimer(s.startTimeout)
	defer deadline.Stop()
	for {
		if owner, found, err := s.read(spec); err == nil && found && owner.PID == pid && !owner.Starting() {
			return nil
		}
		select {
		case err := <-exited:
			detail := ""
			if err != nil {
				detail = ": " + err.Error()
			}
			if tail := lastLine(spec.StderrLogPath); tail != "" {
				detail += ": " + tail
			}
			return fmt.Errorf("supervisor: setsid start: child %d exited before it was ready%s", pid, detail)
		case <-ctx.Done():
			return fmt.Errorf("supervisor: setsid start: child %d: %w", pid, ctx.Err())
		case <-deadline.C:
			// Timed out, but the child may hold the lock by now and have
			// merely been slow to publish its endpoint: a lock held by
			// our child (its PID, and not the pre-spawn acquisition) is
			// a success, not an orphan.
			if owner, found, err := s.read(spec); err == nil && found &&
				owner.PID == pid && (!prevFound || !sameOwner(prev, owner)) {
				return nil
			}
			return fmt.Errorf("supervisor: setsid start: child %d not ready within %s", pid, s.startTimeout)
		case <-time.After(pollInterval):
		}
	}
}

// sameOwner reports whether two lock readings describe the same
// acquisition: the PID plus boot id and start time must all match, so a
// reused PID fails the comparison.
func sameOwner(want, have proclock.Owner) bool {
	return want.PID == have.PID && want.BootID == have.BootID && want.StartedAt.Equal(have.StartedAt)
}

// Stop signals the lock owner SIGTERM, then SIGKILL after the grace.
// Before each signal the lock is re-read and the owner's identity
// verified against the acquisition we decided to signal, so a PID reused
// by an unrelated process is never signaled. A missing, dead or self
// owner is a no-op; a changed owner aborts the signal chain.
func (s *Setsid) Stop(ctx context.Context, spec supervisor.Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner, found, err := s.read(spec)
	if err != nil {
		return fmt.Errorf("supervisor: setsid stop: %w", err)
	}
	if !found || owner.PID <= 0 || owner.PID == os.Getpid() || !pidAlive(owner.PID) {
		return nil
	}
	return s.terminate(ctx, spec, owner)
}

// Status reports the lock owner. A live owner counts as installed: it
// outlives every CLI process, unlike the in-memory Install record.
func (s *Setsid) Status(_ context.Context, spec supervisor.Spec) (supervisor.Status, error) {
	s.mu.Lock()
	st := supervisor.Status{Mode: Mode, Installed: s.installed[spec.UnitName]}
	s.mu.Unlock()
	owner, found, err := s.read(spec)
	if err != nil {
		return st, fmt.Errorf("supervisor: setsid status: %w", err)
	}
	if !found || owner.PID <= 0 {
		return st, nil
	}
	if !pidAlive(owner.PID) {
		st.Diagnostic = "stale owner (pid not alive)"
		return st, nil
	}
	st.Installed, st.Running, st.PID = true, true, owner.PID
	return st, nil
}

func (s *Setsid) read(spec supervisor.Spec) (proclock.Owner, bool, error) {
	return proclock.Read(s.lockOptions(spec)) //nolint:wrapcheck // carries the proclock prefix
}

// stillOwner re-reads the lock immediately before each signal and
// reports whether the acquisition we decided to signal still holds it;
// found=false counts as a mismatch.
func (s *Setsid) stillOwner(spec supervisor.Spec, want proclock.Owner) (bool, error) {
	have, found, err := s.read(spec)
	if err != nil {
		return false, fmt.Errorf("supervisor: setsid stop: %w", err)
	}
	return found && sameOwner(want, have), nil
}

func (s *Setsid) terminate(ctx context.Context, spec supervisor.Spec, owner proclock.Owner) error {
	if ok, err := s.stillOwner(spec, owner); err != nil {
		return err
	} else if !ok {
		return nil
	}
	if err := syscall.Kill(owner.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("supervisor: setsid stop: signal %d: %w", owner.PID, err)
	}
	if waitGone(ctx, owner.PID, s.killGrace) {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("supervisor: setsid stop: pid %d: %w", owner.PID, ctx.Err())
	}
	if ok, err := s.stillOwner(spec, owner); err != nil {
		return err
	} else if !ok {
		// The owner released while dying; it may still be exiting, but
		// the lock no longer identifies it as the victim.
		return nil
	}
	if err := syscall.Kill(owner.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("supervisor: setsid stop: kill %d: %w", owner.PID, err)
	}
	if !waitGone(ctx, owner.PID, sigkillReapBudget) {
		return fmt.Errorf("supervisor: setsid stop: pid %d survived SIGKILL", owner.PID)
	}
	return nil
}

// waitGone polls until pid is gone, budget elapses or ctx ends.
func waitGone(ctx context.Context, pid int, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for pidAlive(pid) {
		if !time.Now().Before(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(pollInterval):
		}
	}
	return true
}

// pidAlive treats EPERM as alive: the process exists, we just may not
// signal it.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// lastLine returns the last non-empty line of path, "" when unreadable.
func lastLine(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
