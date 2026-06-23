package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// CmdExecutor runs external commands so backends can be tested without
// systemctl or launchctl. A non-zero exit returns an error wrapping
// *exec.ExitError alongside the captured output.
type CmdExecutor interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

// OSExecutor is the os/exec-backed CmdExecutor.
type OSExecutor struct{}

// NewOSExecutor returns an OSExecutor.
func NewOSExecutor() *OSExecutor { return &OSExecutor{} }

var _ CmdExecutor = (*OSExecutor)(nil)

// Run executes name with args, capturing stdout and stderr separately.
func (e *OSExecutor) Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), errb.Bytes(), fmt.Errorf("supervisor: run %q: %w", name, err)
	}
	return out.Bytes(), errb.Bytes(), nil
}
