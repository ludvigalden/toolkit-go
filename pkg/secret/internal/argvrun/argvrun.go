// Package argvrun runs an argv without a shell and returns its stdout
// minus trailing whitespace, folding stderr into the error on failure.
package argvrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Output runs argv in dir with env (nil inherits). On failure stdout is
// discarded and stderr becomes part of the error.
func Output(ctx context.Context, argv, env []string, dir string) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("command source has no argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env, cmd.Dir = env, dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s: %w: %s", argv[0], err, msg)
		}
		return "", fmt.Errorf("%s: %w", argv[0], err)
	}
	return strings.TrimRight(stdout.String(), " \t\r\n"), nil
}
