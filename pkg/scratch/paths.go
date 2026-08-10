package scratch

import (
	"fmt"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/scratch/internal/pathsafe"
)

// Resolve validates rel as a root-relative path and returns its
// absolute location inside the root. Absolute paths and any ".."
// element are rejected (ErrTraversal); symlink components inside the
// root are rejected (ErrSymlinkEscape) as they exist at validation
// time.
func (e *Env) Resolve(rel string) (string, error) {
	abs, err := pathsafe.ResolveAbs(e.root, rel)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", rel, err)
	}
	return abs, nil
}

// Mkdir creates rel (and parents) inside the root with 0700
// permissions, refusing traversal and symlinked components.
func (e *Env) Mkdir(rel string) error {
	if err := pathsafe.MkdirAll(e.root, rel); err != nil {
		return fmt.Errorf("mkdir %q: %w", rel, err)
	}
	return nil
}

// WriteFile writes content to rel inside the root, creating parent
// directories with 0700 permissions, refusing traversal, rejecting
// symlinked components as they exist at validation time, and opening
// the final component with O_NOFOLLOW. perm zero defaults to 0644.
func (e *Env) WriteFile(rel string, content []byte, perm os.FileMode) error {
	if err := pathsafe.WriteFile(e.root, rel, content, perm); err != nil {
		return fmt.Errorf("write %q: %w", rel, err)
	}
	return nil
}
