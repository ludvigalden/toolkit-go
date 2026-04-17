//go:build linux || darwin

package diskfree

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// LowDiskError reports that a volume holds less free space than required.
type LowDiskError struct {
	Path string
	Free uint64
	Need uint64
}

// Error implements error.
func (e *LowDiskError) Error() string {
	return fmt.Sprintf("diskfree: %s has %d bytes free, need %d", e.Path, e.Free, e.Need)
}

// Free reports the bytes available to unprivileged users on the volume
// holding path. A missing final component resolves through its parent, so
// callers can check before creating a file.
func Free(path string) (uint64, error) {
	return statFree(path)
}

// Check returns a *LowDiskError when the volume holding path has fewer than
// need bytes free. Measurement failures are returned unwrapped.
func Check(path string, need uint64) error {
	return check(statFree, path, need)
}

// check is Check with the measurement injected.
func check(stat func(string) (uint64, error), path string, need uint64) error {
	free, err := stat(path)
	if err != nil {
		return err
	}
	if free < need {
		return &LowDiskError{Path: path, Free: free, Need: need}
	}
	return nil
}

// statFree runs statfs(2) on path, or on its parent when path is absent.
func statFree(path string) (uint64, error) {
	target := path
	if _, err := os.Stat(path); err != nil {
		parent := filepath.Dir(path)
		if _, perr := os.Stat(parent); perr != nil {
			return 0, fmt.Errorf("diskfree: stat %s: %w", path, err)
		}
		target = parent
	}
	var st unix.Statfs_t
	if err := unix.Statfs(target, &st); err != nil {
		return 0, fmt.Errorf("diskfree: statfs %s: %w", target, err)
	}
	return st.Bavail * uint64(st.Bsize), nil //nolint:gosec // Bsize is positive
}
