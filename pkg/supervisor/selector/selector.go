// Package selector picks the platform supervisor backend: systemd-user on
// Linux, launchd on macOS, the setsid fallback when neither is usable.
package selector

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/ludvigalden/toolkit-go/pkg/proclock"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/setsid"
)

// Mode names the selected backend.
type Mode string

// Selection modes.
const (
	ModeSystemd Mode = "systemd"
	ModeLaunchd Mode = "launchd"
	ModeSetsid  Mode = "setsid"
	// ModeBypass marks a Wrap construction: no selection happened.
	ModeBypass Mode = "bypass"
)

// Options carries the settings every backend may need.
type Options struct {
	// LockDir is the proclock root of the setsid fallback. Exactly one
	// of LockDir and LockOptions is required, so selection behaves the
	// same on every host.
	LockDir string
	// LockOptions locates each service's lock for the setsid fallback
	// (see setsid.WithLockOptions).
	LockOptions func(supervisor.Spec) proclock.Options
	// Getenv reads HOME, USER and the XDG variables (nil: os.Getenv).
	Getenv func(string) string
	// Markers receives backend maintenance markers (zero: disabled).
	Markers supervisor.Markers
	// LogDir holds derived launchd per-unit logs (empty: the default).
	LogDir string
	// Logger receives best-effort backend failures (nil: discard).
	Logger *slog.Logger
}

// Selected is the chosen backend plus why it was chosen. The caller
// decides whether and where to print DiagnosticLine.
type Selected struct {
	supervisor.Supervisor
	mode   Mode
	reason string
}

// Mode returns the selected backend's mode.
func (s *Selected) Mode() Mode { return s.mode }

// Reason explains the selection.
func (s *Selected) Reason() string { return s.reason }

// DiagnosticLine is a newline-terminated one-line summary for stderr, or
// "" for ModeBypass.
func (s *Selected) DiagnosticLine() string {
	if s.mode == ModeBypass {
		return ""
	}
	return fmt.Sprintf("supervisor: %s (selected because: %s)\n", s.mode, s.reason)
}

// Wrap returns b as a Selected without probing, for tests and callers
// that choose the backend themselves.
func Wrap(b supervisor.Supervisor) *Selected {
	return &Selected{Supervisor: b, mode: ModeBypass, reason: "explicit backend"}
}

func checkOptions(exec supervisor.CmdExecutor, o Options) error {
	if exec == nil {
		return errors.New("supervisor: selector: executor is nil")
	}
	if (o.LockDir == "") == (o.LockOptions == nil) {
		return errors.New("supervisor: selector: exactly one of Options.LockDir and Options.LockOptions is required")
	}
	return nil
}

func newSetsid(o Options) (*setsid.Setsid, error) {
	var opts []setsid.Option
	if o.LockOptions != nil {
		opts = append(opts, setsid.WithLockOptions(o.LockOptions))
	}
	s, err := setsid.New(o.LockDir, opts...)
	if err != nil {
		return nil, fmt.Errorf("supervisor: selector: %w", err)
	}
	return s, nil
}
