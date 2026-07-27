// Package pathsafe validates root-relative paths, contains symlinks,
// and copies fixtures into a scratch root: paths are checked lexically,
// symlinks are rejected on sight, and the final open carries O_NOFOLLOW.
package pathsafe

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var (
	// ErrTraversal reports a path that would leave the scratch root:
	// an absolute path, a ".." element, or the root itself.
	ErrTraversal = errors.New("path escapes the scratch root")

	// ErrSymlinkEscape reports a symlinked path component inside the
	// scratch root through which a write could leave the root.
	ErrSymlinkEscape = errors.New("symlink inside the scratch root")
)

// scratchDirMode is the permission every directory created inside the
// root receives: 0700, so other local users cannot read scratch
// content.
const scratchDirMode = os.FileMode(0o700)

// defaultFilePerm is used when a fixture carries no usable permission
// bits (some fs.FS implementations report zero).
const defaultFilePerm = os.FileMode(0o644)

// ValidateRel cleans and validates a root-relative path, returning the
// cleaned form or an error wrapping ErrTraversal.
func ValidateRel(rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("%w: empty path", ErrTraversal)
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: absolute path %q", ErrTraversal, rel)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrTraversal, rel)
	}
	if clean == "." {
		return "", fmt.Errorf("%w: %q names the root itself", ErrTraversal, rel)
	}
	return clean, nil
}

// ResolveAbs validates rel and returns its absolute location inside
// root. Existing components are checked for symlinks; components that
// do not exist yet are accepted (they will be created inside the root).
func ResolveAbs(root, rel string) (string, error) {
	clean, err := ValidateRel(rel)
	if err != nil {
		return "", err
	}
	if err := checkComponents(root, clean); err != nil {
		return "", err
	}
	return filepath.Join(root, clean), nil
}

// MkdirAll creates rel below root as directories (0700), refusing
// traversal and symlinked components.
func MkdirAll(root, rel string) error {
	clean, err := ValidateRel(rel)
	if err != nil {
		return err
	}
	return ensureDirs(root, splitRel(clean))
}

// WriteFile writes content to rel below root, creating parent
// directories. Every component is symlink-checked and the final open
// carries O_NOFOLLOW, so a symlink cannot redirect the write outside
// the root.
func WriteFile(root, rel string, content []byte, perm os.FileMode) error {
	clean, err := ValidateRel(rel)
	if err != nil {
		return err
	}
	parts := splitRel(clean)
	if err := ensureDirs(root, parts[:len(parts)-1]); err != nil {
		return err
	}
	abs := filepath.Join(root, clean)
	fi, err := os.Lstat(abs)
	switch {
	case err == nil:
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %q", ErrSymlinkEscape, abs)
		}
		if fi.IsDir() {
			return fmt.Errorf("write %q: is a directory", abs)
		}
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect %q: %w", abs, err)
	}
	if perm == 0 {
		perm = defaultFilePerm
	}
	return writeContent(abs, content, perm)
}

// CopyPath copies the file or directory tree at host path src into the
// root at dst. Copies are ordinary files — never symlinks into the
// source — so the seeded scenario cannot mutate the canonical source.
// Symlinked or special source entries are rejected.
func CopyPath(root, dst, src string) error {
	cleanDst, err := ValidateRel(dst)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("inspect fixture source %q: %w", src, err)
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("fixture source %q is a symlink", src)
	case fi.IsDir():
		return copyDir(root, cleanDst, src)
	case fi.Mode().IsRegular():
		return copyFileFromPath(root, cleanDst, src, fi.Mode().Perm())
	default:
		return fmt.Errorf("fixture source %q: unsupported type", src)
	}
}

// CopyFS copies the fsys subtree at src into the root at dst, with the
// same copy semantics as CopyPath: ordinary files only, symlinks
// rejected. fsys is typically an embed.FS carrying embedded fixtures.
func CopyFS(root, dst string, fsys fs.FS, src string) error {
	cleanDst, err := ValidateRel(dst)
	if err != nil {
		return err
	}
	if !fs.ValidPath(src) {
		return fmt.Errorf("fixture source %q: invalid fs path", src)
	}
	return fs.WalkDir(fsys, src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk fixture fs at %q: %w", src, err)
		}
		dstRel := cleanDst
		if p != src {
			sub := strings.TrimPrefix(strings.TrimPrefix(p, src), "/")
			dstRel = filepath.Join(cleanDst, sub)
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: fixture entry %q is a symlink", ErrSymlinkEscape, p)
		case d.IsDir():
			return ensureDirsAll(root, dstRel)
		case d.Type().IsRegular():
			return copyFileFromFS(root, dstRel, fsys, p)
		default:
			return fmt.Errorf("fixture entry %q: unsupported type", p)
		}
	})
}

// splitRel splits an already-validated clean relative path into
// components. Clean paths never begin with a separator.
func splitRel(clean string) []string {
	return strings.Split(clean, string(filepath.Separator))
}

// ensureDirsAll validates rel then creates its full chain as
// directories inside root.
func ensureDirsAll(root, rel string) error {
	clean, err := ValidateRel(rel)
	if err != nil {
		return err
	}
	return ensureDirs(root, splitRel(clean))
}

// ensureDirs creates each component of parts below root as a
// directory, refusing symlinked or non-directory components.
func ensureDirs(root string, parts []string) error {
	cur := root
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if mkErr := os.Mkdir(cur, scratchDirMode); mkErr != nil {
				return fmt.Errorf("create %q: %w", cur, mkErr)
			}
		case err != nil:
			return fmt.Errorf("inspect %q: %w", cur, err)
		case fi.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%w: %q", ErrSymlinkEscape, cur)
		case !fi.IsDir():
			return fmt.Errorf("create %q: not a directory", cur)
		}
	}
	return nil
}

// checkComponents walks clean's components below root and rejects any
// existing symlink, stopping at the first missing component.
func checkComponents(root, clean string) error {
	cur := root
	for _, part := range splitRel(clean) {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil
		case err != nil:
			return fmt.Errorf("inspect %q: %w", cur, err)
		case fi.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%w: %q", ErrSymlinkEscape, cur)
		case !fi.IsDir():
			return fmt.Errorf("resolve %q: not a directory", cur)
		}
	}
	return nil
}

// writeContent opens abs with O_NOFOLLOW and writes content.
func writeContent(abs string, content []byte, perm os.FileMode) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC | syscall.O_NOFOLLOW
	f, err := os.OpenFile(abs, flags, perm)
	if err != nil {
		return fmt.Errorf("open %q: %w", abs, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %q: %w", abs, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %q: %w", abs, err)
	}
	return nil
}

// copyDir copies the directory tree at srcDir into root/dstClean.
func copyDir(root, dstClean, srcDir string) error {
	if err := ensureDirs(root, splitRel(dstClean)); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk fixture source %q: %w", srcDir, err)
		}
		if p == srcDir {
			return nil
		}
		sub := strings.TrimPrefix(strings.TrimPrefix(p, srcDir), string(filepath.Separator))
		dstRel := filepath.Join(dstClean, sub)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: fixture entry %q is a symlink", ErrSymlinkEscape, p)
		case d.IsDir():
			return ensureDirsAll(root, dstRel)
		case d.Type().IsRegular():
			info, infoErr := d.Info()
			if infoErr != nil {
				return fmt.Errorf("inspect fixture entry %q: %w", p, infoErr)
			}
			return copyFileFromPath(root, dstRel, p, info.Mode().Perm())
		default:
			return fmt.Errorf("fixture entry %q: unsupported type", p)
		}
	})
}

// copyFileFromPath copies one host file into root at dstClean.
func copyFileFromPath(root, dstClean, src string, perm os.FileMode) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open fixture source %q: %w", src, err)
	}
	defer func() { _ = f.Close() }()
	return copyFromReader(root, dstClean, f, perm)
}

// copyFileFromFS copies one fs.FS file into root at dstClean.
func copyFileFromFS(root, dstClean string, fsys fs.FS, name string) error {
	var perm os.FileMode
	if info, err := fs.Stat(fsys, name); err == nil {
		// embed.FS reports 0444; force user-writable so a later
		// WriteFile to the same rel is not blocked by the seed's mode.
		perm = info.Mode().Perm()&0o777 | 0o200
	}
	f, err := fsys.Open(name)
	if err != nil {
		return fmt.Errorf("open fixture entry %q: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	return copyFromReader(root, dstClean, f, perm)
}

// copyFromReader ensures dstClean's parents then streams r into it.
func copyFromReader(root, dstClean string, r io.Reader, perm os.FileMode) error {
	parts := splitRel(dstClean)
	if err := ensureDirs(root, parts[:len(parts)-1]); err != nil {
		return err
	}
	abs := filepath.Join(root, dstClean)
	if fi, err := os.Lstat(abs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %q", ErrSymlinkEscape, abs)
	}
	if perm == 0 {
		perm = defaultFilePerm
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC | syscall.O_NOFOLLOW
	out, err := os.OpenFile(abs, flags, perm)
	if err != nil {
		return fmt.Errorf("open %q: %w", abs, err)
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy into %q: %w", abs, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", abs, err)
	}
	return nil
}
