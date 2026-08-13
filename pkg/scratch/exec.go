package scratch

import (
	"context"
	"os/exec"

	"github.com/ludvigalden/toolkit-go/pkg/scratch/internal/prockill"
)

// CommandContext builds an exec.Cmd that sees only the fenced homes (Dir
// and ChildEnv). Cancellation SIGKILLs the child's whole process group,
// grandchildren included, so Cleanup after Wait cannot race a live
// child. Callers must Wait (directly or via Run) before Cleanup.
func (e *Env) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = e.workDir
	cmd.Env = e.childEnv
	prockill.Configure(cmd)
	return cmd
}
