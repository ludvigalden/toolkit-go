package xdg

import (
	"fmt"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/xdg/internal/atomicfile"
	"github.com/ludvigalden/toolkit-go/pkg/xdg/internal/dirmode"
)

// ErrNotDir is returned when a path that must be a directory is not.
var ErrNotDir = dirmode.ErrNotDir

// Drift describes an existing directory whose permission bits differ
// from the requested mode.
type Drift struct {
	Path   string
	Want   os.FileMode
	Actual os.FileMode
}

// String renders the drift for a diagnostic.
func (d Drift) String() string {
	return fmt.Sprintf("%s has mode %04o, want %04o", d.Path, d.Actual, d.Want)
}

// EnsureDir creates dir with mode's permission bits when absent. An
// existing directory is never chmod'ed: a mismatch is returned as Drift
// so the caller can report it or call RepairDir deliberately.
func EnsureDir(dir string, mode os.FileMode) (*Drift, error) {
	actual, err := dirmode.Ensure(dir, mode.Perm())
	if err != nil {
		return nil, err //nolint:wrapcheck // carries the xdg prefix
	}
	if actual != mode.Perm() {
		return &Drift{Path: dir, Want: mode.Perm(), Actual: actual}, nil
	}
	return nil, nil
}

// RepairDir chmods an existing directory to mode's permission bits.
// Only the owner or root may do so; the kernel's refusal is returned.
func RepairDir(dir string, mode os.FileMode) error {
	return dirmode.Repair(dir, mode.Perm()) //nolint:wrapcheck // carries the xdg prefix
}

// WriteFileAtomic replaces dir/name with data at mode via a temp file
// in dir that is chmod'ed, fsynced and renamed into place. Readers see
// the old or new file, never a partial one. name must be a bare name.
func WriteFileAtomic(dir, name string, data []byte, mode os.FileMode) error {
	return atomicfile.Write(dir, name, data, mode.Perm()) //nolint:wrapcheck // carries the xdg prefix
}
