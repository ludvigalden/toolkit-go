// Package atomicfile replaces a file via a chmod'ed, fsynced temp file
// renamed into place, so readers never see a partial write.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Write replaces dir/name with data at perm. name must be a bare name.
func Write(dir, name string, data []byte, perm os.FileMode) (err error) {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, os.PathSeparator) {
		return fmt.Errorf("xdg: %q is not a bare file name", name)
	}
	tmp, err := os.CreateTemp(dir, "."+name+".tmp-*")
	if err != nil {
		return fmt.Errorf("xdg: create temp in %s: %w", dir, err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()           //nolint:errcheck // already failing
			_ = os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("xdg: write %s: %w", tmp.Name(), err)
	}
	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("xdg: chmod %s: %w", tmp.Name(), err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("xdg: fsync %s: %w", tmp.Name(), err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("xdg: close %s: %w", tmp.Name(), err)
	}
	if err = os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("xdg: rename into %s: %w", dir, err)
	}
	return nil
}
