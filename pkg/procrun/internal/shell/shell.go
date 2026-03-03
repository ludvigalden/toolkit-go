// Package shell builds POSIX shell command lines and probes a user's
// shell for a binary that is not on PATH.
package shell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// probeTimeout bounds the shell probe: a hung shell init must not block
// Find forever.
const probeTimeout = 5 * time.Second

// Quote single-quotes arg for a POSIX shell command line.
func Quote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// Line joins name and its quoted args into one "-c" script; name stays
// unquoted so the shell resolves it through functions and aliases.
func Line(name string, args []string) string {
	words := make([]string, len(args)+1)
	words[0] = name
	for i, a := range args {
		words[i+1] = Quote(a)
	}
	return strings.Join(words, " ")
}

// Probe runs `shell -c "command -v name"` with env; prepare arms the
// command (for example with a process group) before it starts. Only an
// absolute result counts.
func Probe(shell, name string, env []string, prepare func(*exec.Cmd)) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-c", "command -v "+Quote(name))
	prepare(cmd)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("shell probe: %w", err)
	}
	resolved := strings.TrimSpace(string(out))
	if !filepath.IsAbs(resolved) {
		return "", errors.New("non-absolute shell probe result")
	}
	return resolved, nil
}
