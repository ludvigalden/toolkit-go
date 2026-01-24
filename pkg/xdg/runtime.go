package xdg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ludvigalden/toolkit-go/pkg/xdg/internal/envpath"
	"github.com/ludvigalden/toolkit-go/pkg/xdg/internal/privdir"
)

// RuntimeDirFor returns app's runtime directory for sockets and locks:
// $XDG_RUNTIME_DIR/<app> when that variable is absolute, otherwise
// FallbackRuntimeDir. The directory is created with mode 0700.
func RuntimeDirFor(app string, getenv func(string) string) (string, error) {
	if err := privdir.CheckName(app); err != nil {
		return "", err //nolint:wrapcheck // carries the xdg prefix
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if rd := envpath.Abs(getenv("XDG_RUNTIME_DIR")); rd != "" {
		dir := filepath.Join(rd, app)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("xdg: runtime dir: %w", err)
		}
		return dir, nil
	}
	return FallbackRuntimeDir(app)
}

// FallbackRuntimeDir returns /tmp/<app>-<uid>, created 0700. Because
// /tmp is shared, it refuses a directory that is a symlink, is owned by
// another user, or is accessible to group or others.
func FallbackRuntimeDir(app string) (string, error) {
	if err := privdir.CheckName(app); err != nil {
		return "", err //nolint:wrapcheck // carries the xdg prefix
	}
	return privdir.Ensure(os.TempDir(), app) //nolint:wrapcheck // carries the xdg prefix
}

// RuntimeDirs lists app's existing runtime directories, preferred
// first, so a scanner finds entries created under either location.
func RuntimeDirs(app string, getenv func(string) string) []string {
	if privdir.CheckName(app) != nil {
		return nil
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	var cands []string
	if rd := envpath.Abs(getenv("XDG_RUNTIME_DIR")); rd != "" {
		cands = append(cands, filepath.Join(rd, app))
	}
	cands = append(cands, privdir.UserPath(os.TempDir(), app))
	var out []string
	for _, c := range cands {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			out = append(out, c)
		}
	}
	return out
}
