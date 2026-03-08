package procrun

import (
	"fmt"
	"strings"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/noise"
)

// StripNoise drops blank lines and the lines a wrapping shell's init (zsh
// zle warnings) or a systemd-run scope launcher writes to stderr ahead of
// the subprocess's own output. It returns nil when nothing remains.
func StripNoise(stderr []byte) []byte { return noise.Strip(stderr) }

// ToolNotFoundError reports that a binary could not be resolved.
type ToolNotFoundError struct {
	Name string
	// Hint is an optional install hint appended to the message.
	Hint string
}

// Error implements error.
func (e *ToolNotFoundError) Error() string {
	msg := fmt.Sprintf("tool %q not found", e.Name)
	if e.Hint != "" {
		msg += " (" + e.Hint + ")"
	}
	return msg
}

// ExitError reports a non-zero exit. Stderr is the raw captured stderr.
type ExitError struct {
	Name     string
	ExitCode int
	Stderr   []byte
	Err      error
}

// Error implements error. StripNoise is applied to the quoted stderr;
// e.Stderr keeps the raw bytes.
func (e *ExitError) Error() string {
	msg := fmt.Sprintf("%s exited with code %d", e.Name, e.ExitCode)
	if clean := StripNoise(e.Stderr); len(clean) > 0 {
		msg += ": " + strings.TrimSpace(string(clean))
	}
	return msg
}

// Unwrap supports errors.Is and errors.As.
func (e *ExitError) Unwrap() error { return e.Err }

// TimeoutError reports a subprocess killed by its timeout. Grace is the
// SIGTERM window granted before SIGKILL (zero: SIGKILL first).
type TimeoutError struct {
	Name    string
	Timeout string
	Grace   time.Duration
	Err     error
}

// Error implements error.
func (e *TimeoutError) Error() string {
	msg := fmt.Sprintf("%s timed out after %s", e.Name, e.Timeout)
	if e.Grace > 0 {
		msg += fmt.Sprintf(" (graceful stop, SIGKILL after %v)", e.Grace)
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap supports errors.Is and errors.As.
func (e *TimeoutError) Unwrap() error { return e.Err }

// DoubleWaitError reports a second Wait on one Process.
type DoubleWaitError struct {
	Name string
}

// Error implements error.
func (e *DoubleWaitError) Error() string {
	return fmt.Sprintf("wait already called on process %s", e.Name)
}
