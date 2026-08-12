package scratch

import (
	"fmt"
	"io/fs"

	"github.com/ludvigalden/toolkit-go/pkg/scratch/internal/pathsafe"
)

// Seed copies a caller-provided fixture — a file or a directory tree at
// host path src — into the root at dst. The copy consists of ordinary
// files, never symlinks into the source, so the scenario cannot mutate
// the canonical fixture source. Symlinked source entries are rejected.
func (e *Env) Seed(dst, src string) error {
	if err := pathsafe.CopyPath(e.root, dst, src); err != nil {
		return fmt.Errorf("seed %q from %q: %w", dst, src, err)
	}
	return nil
}

// SeedFS copies a fixture subtree from an fs.FS (for example an
// embed.FS carrying embedded fixtures) into the root at dst, with the
// same copy-on-seed semantics as Seed.
func (e *Env) SeedFS(dst string, fsys fs.FS, src string) error {
	if err := pathsafe.CopyFS(e.root, dst, fsys, src); err != nil {
		return fmt.Errorf("seed %q from fs at %q: %w", dst, src, err)
	}
	return nil
}
