// Package privdir creates per-user directories in shared locations and
// refuses ones that are symlinks, foreign-owned, or open to others.
package privdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// CheckName keeps name a single path segment.
func CheckName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, os.PathSeparator) {
		return fmt.Errorf("xdg: invalid app name %q", name)
	}
	return nil
}

// UserPath returns base/<name>-<uid>.
func UserPath(base, name string) string {
	return filepath.Join(base, name+"-"+strconv.Itoa(os.Getuid()))
}

// Ensure creates UserPath(base, name) 0700 and verifies it is a real
// directory owned by this user with no group or other access.
func Ensure(base, name string) (string, error) {
	dir := UserPath(base, name)
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("xdg: fallback runtime dir: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("xdg: fallback runtime dir: %w", err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(st.Uid) != os.Getuid() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("xdg: fallback runtime dir %s must be a 0700 directory owned by uid %d", dir, os.Getuid())
	}
	return dir, nil
}
