// Package fencing owns the scratch-root layout, the six-variable child
// environment, and the teardown policy (remove by default, retain on
// failure). XDG homes sit at their HOME-relative defaults; nothing
// falls through to the host.
package fencing

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/scratch/internal/pathsafe"
)

// Layout defaults overridable through the scratch option set.
const (
	DefaultPrefix  = "scratch-"
	DefaultWorkRel = "work"
)

// FencedVar pairs an environment variable key with its root-relative
// value.
type FencedVar struct {
	Key string
	Rel string
}

// coreVars is the fixed six-variable fencing table; the XDG homes sit
// at their HOME-relative defaults under the fenced HOME.
var coreVars = []FencedVar{
	{Key: "HOME", Rel: "home"},
	{Key: "TMPDIR", Rel: "tmp"},
	{Key: "XDG_CONFIG_HOME", Rel: "home/.config"},
	{Key: "XDG_CACHE_HOME", Rel: "home/.cache"},
	{Key: "XDG_DATA_HOME", Rel: "home/.local/share"},
	{Key: "XDG_STATE_HOME", Rel: "home/.local/state"},
}

// Config carries the constructor options resolved by the scratch
// package.
type Config struct {
	Parent          string
	Prefix          string
	WorkRel         string
	Extras          []FencedVar
	Stderr          io.Writer
	RetainOnFailure bool
	// Environ supplies the base environment snapshot; nil means
	// os.Environ.
	Environ func() []string
	// TempDir supplies the default root parent; nil means os.TempDir.
	TempDir func() string
}

// State is the created-environment outcome handed back to the owner.
type State struct {
	Root     string
	WorkDir  string
	ChildEnv []string
}

// diagWriter is the default notice sink: each Write emits one slog.Warn
// record with the trailing newline stripped, so the notice follows the
// process-wide log configuration.
type diagWriter struct{}

func (diagWriter) Write(p []byte) (int, error) {
	slog.Warn(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// Create builds the scratch root under cfg.Parent (default: the system
// temporary directory), lays out the fenced directories, and assembles
// the child environment. Setup failures remove partial state by
// default; with RetainOnFailure the partial root is kept and its path
// printed through the notice sink before the error is returned.
func Create(cfg Config) (State, error) {
	if cfg.Prefix == "" {
		cfg.Prefix = DefaultPrefix
	}
	if cfg.WorkRel == "" {
		cfg.WorkRel = DefaultWorkRel
	}
	if cfg.Stderr == nil {
		cfg.Stderr = diagWriter{}
	}
	if cfg.Environ == nil {
		cfg.Environ = os.Environ
	}
	if cfg.TempDir == nil {
		cfg.TempDir = os.TempDir
	}
	parent := cfg.Parent
	if parent == "" {
		parent = cfg.TempDir()
	}

	root, err := os.MkdirTemp(parent, cfg.Prefix)
	if err != nil {
		return State{}, fmt.Errorf("create scratch root: %w", err)
	}
	fail := func(cause error) (State, error) {
		if cfg.RetainOnFailure {
			_, _ = fmt.Fprintf(cfg.Stderr,
				"scratch: retained after setup failure: %s\n", root)
			return State{}, fmt.Errorf("%w (scratch retained at %s)", cause, root)
		}
		if rmErr := os.RemoveAll(root); rmErr != nil {
			return State{}, fmt.Errorf("%w (partial cleanup failed: %v)", cause, rmErr)
		}
		return State{}, cause
	}

	for _, v := range coreVars {
		if err := pathsafe.MkdirAll(root, v.Rel); err != nil {
			return fail(fmt.Errorf("create fenced %s directory: %w", v.Key, err))
		}
	}
	if err := pathsafe.MkdirAll(root, cfg.WorkRel); err != nil {
		return fail(fmt.Errorf("create working directory: %w", err))
	}

	coreKey := make(map[string]bool, len(coreVars))
	for _, v := range coreVars {
		coreKey[v.Key] = true
	}
	seen := make(map[string]bool, len(cfg.Extras))
	for _, v := range cfg.Extras {
		if v.Key == "" {
			return fail(errors.New("fenced var: empty key"))
		}
		if coreKey[v.Key] {
			return fail(fmt.Errorf("fenced var %s: cannot redefine a core fenced variable", v.Key))
		}
		if seen[v.Key] {
			return fail(fmt.Errorf("fenced var %s: declared twice", v.Key))
		}
		seen[v.Key] = true
		if err := pathsafe.MkdirAll(root, v.Rel); err != nil {
			return fail(fmt.Errorf("fenced var %s: %w", v.Key, err))
		}
	}

	env := buildChildEnv(cfg.Environ(), root, coreKey, seen, cfg.Extras)
	workAbs, err := pathsafe.ResolveAbs(root, cfg.WorkRel)
	if err != nil {
		return fail(fmt.Errorf("resolve working directory: %w", err))
	}
	return State{Root: root, WorkDir: workAbs, ChildEnv: env}, nil
}

// Remove tears the scratch root down. os.RemoveAll unlinks symlinks
// rather than following them, so deletion cannot be redirected outside
// the root. Removing an already-absent root succeeds.
func Remove(root string) error {
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove scratch root %q: %w", root, err)
	}
	return nil
}

// buildChildEnv snapshots the constructing process environment, drops
// every fenced key, and appends the fenced values inside the root.
func buildChildEnv(base []string, root string, coreKey, extraKey map[string]bool, extras []FencedVar) []string {
	env := make([]string, 0, len(base)+len(coreVars)+len(extras))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (coreKey[key] || extraKey[key]) {
			continue
		}
		env = append(env, entry)
	}
	for _, v := range coreVars {
		env = append(env, v.Key+"="+filepath.Join(root, v.Rel))
	}
	for _, v := range extras {
		env = append(env, v.Key+"="+filepath.Join(root, v.Rel))
	}
	return env
}
