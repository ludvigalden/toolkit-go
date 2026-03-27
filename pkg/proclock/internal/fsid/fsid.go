// Package fsid derives filesystem identities so that every spelling of
// one directory (symlinks, bind mounts) maps to one cache key, and two
// distinct directories never share one.
package fsid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Segment returns a stable cache segment for root: the home-relative
// spelling of its symlink-resolved path ("~/Code/repo"), or the absolute
// path outside home. Distinct roots never collide.
func Segment(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", root, err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if r, err := filepath.EvalSymlinks(home); err == nil {
			home = r
		}
		if abs == home {
			return "~", nil
		}
		if rest, ok := strings.CutPrefix(abs, home+string(filepath.Separator)); ok {
			return "~/" + rest, nil
		}
	}
	return abs, nil
}
