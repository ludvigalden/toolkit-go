package supervisor

import (
	"context"
	"errors"
)

// ErrUnsupportedPlatform is returned on hosts other than Linux and macOS.
var ErrUnsupportedPlatform = errors.New("supervisor: unsupported platform (only linux and darwin)")

// Supervisor manages one supervised service. Restart is Stop then Start
// and belongs to the caller.
type Supervisor interface {
	// Install provisions the unit or plist (setsid records intent).
	// Idempotent.
	Install(ctx context.Context, spec Spec) error
	// Start launches the service; a running service is a no-op.
	Start(ctx context.Context, spec Spec) error
	// Stop halts the service but leaves it installed. Idempotent.
	Stop(ctx context.Context, spec Spec) error
	// Status reports the current state.
	Status(ctx context.Context, spec Spec) (Status, error)
	// Uninstall removes the unit or plist; call Stop first.
	Uninstall(ctx context.Context, spec Spec) error
}

// Spec is the input to every Supervisor method. The JSON names are a wire
// format; renaming one is a breaking change.
type Spec struct {
	// Namespace is the reverse-DNS prefix of every unit name, for example
	// "com.example".
	Namespace string `json:"namespace"`
	// UnitName is the service's lock name: tool, or tool:N for an
	// instance. The part before the colon is the tool segment.
	UnitName string `json:"unit_name"`
	// ExePath is the absolute path of the binary to run.
	ExePath string `json:"exe_path"`
	// Args is the argv after ExePath.
	Args []string `json:"args"`
	// Port is the TCP port the service binds (0: OS-assigned); it is the
	// last segment of the unit identity.
	Port int `json:"port"`
	// StderrLogPath receives the service's stdout and stderr. Empty means
	// the journal (systemd), a derived per-unit log (launchd), or
	// /dev/null (setsid).
	StderrLogPath string `json:"stderr_log_path"`
	// WorkingDirectory is the absolute directory the service runs in.
	WorkingDirectory string `json:"working_directory"`
	// OOMScoreAdjust overrides the systemd default of -500, which shields
	// small daemons from collateral OOM kills. Other backends ignore it.
	OOMScoreAdjust *int `json:"oom_score_adjust"`
}

// Status is the output of Supervisor.Status.
type Status struct {
	// Installed reports whether the unit is provisioned.
	Installed bool `json:"installed"`
	// Running reports whether the service is active.
	Running bool `json:"running"`
	// PID is the main PID, or 0 when not running.
	PID int `json:"pid"`
	// Mode names the backend: "systemd", "launchd" or "setsid".
	Mode string `json:"mode"`
	// LastExitStatus is the last exit code, "" when unknown.
	LastExitStatus string `json:"last_exit_status"`
	// Diagnostic is backend-specific detail, "" when nothing is notable.
	Diagnostic string `json:"diagnostic"`
}
