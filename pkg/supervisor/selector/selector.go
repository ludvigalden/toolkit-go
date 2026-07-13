// Package selector picks the platform supervisor backend: systemd-user on
// Linux, launchd on macOS, the setsid fallback when neither is usable.
package selector

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
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
	// LockDir is the proclock root of the setsid fallback. Required, so
	// selection behaves the same on every host.
	LockDir string
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
	if o.LockDir == "" {
		return errors.New("supervisor: selector: Options.LockDir is required")
	}
	return nil
}
