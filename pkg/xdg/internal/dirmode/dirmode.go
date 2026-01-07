// Package dirmode creates directories at a permission and reports, or
// deliberately repairs, existing ones whose bits drifted.
package dirmode

import (
	"errors"
	"fmt"
	"os"
)

// ErrNotDir is returned when a path that must be a directory is not.
var ErrNotDir = errors.New("xdg: not a directory")

// Ensure creates dir at perm when absent and returns (perm, nil). An
// existing directory is never chmod'ed; its actual bits are returned.
func Ensure(dir string, perm os.FileMode) (os.FileMode, error) {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, perm); err != nil {
			return 0, fmt.Errorf("xdg: create %s: %w", dir, err)
		}
		return perm, nil
	}
	if err != nil {
		return 0, fmt.Errorf("xdg: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%w: %s", ErrNotDir, dir)
	}
	return info.Mode().Perm(), nil
}

// Repair chmods an existing directory to perm.
func Repair(dir string, perm os.FileMode) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("xdg: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s", ErrNotDir, dir)
	}
	if err := os.Chmod(dir, perm); err != nil {
		return fmt.Errorf("xdg: chmod %s: %w", dir, err)
	}
	return nil
}
