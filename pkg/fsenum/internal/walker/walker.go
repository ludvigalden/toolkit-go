// Package walker is the fs.WalkDir traversal behind every fsenum entry
// point: depth, .git, ignore-file, and Skip pruning, then a per-file
// visit callback.
package walker

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/fsenum/internal/ignore"
)

// Rules selects what a walk visits; see fsenum.Options for semantics.
type Rules struct {
	Skip            func(rel string, isDir bool) bool
	NoIgnore        bool
	MaxDepth        int
	IncludeSymlinks bool
}

// Walk calls visit for every permitted file under fsys. Vanishing paths
// are skipped; visit errors and ctx cancellation end the walk.
func Walk(ctx context.Context, fsys fs.FS, r Rules, visit func(rel string, d fs.DirEntry) error) error {
	var ig *ignore.Matcher
	if !r.NoIgnore {
		ig = ignore.New()
	}
	return fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error { //nolint:wrapcheck // callers wrap
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if rel == "." {
			if ig != nil {
				ig.Load(fsys, rel)
			}
			return nil
		}
		if d.IsDir() {
			if pruneDir(rel, d, r, ig) {
				return fs.SkipDir
			}
			if ig != nil {
				ig.Load(fsys, rel)
			}
			return nil
		}
		if !keepFile(rel, d, r, ig) {
			return nil
		}
		return visit(rel, d)
	})
}

// pruneDir reports whether the walk skips directory rel.
func pruneDir(rel string, d fs.DirEntry, r Rules, ig *ignore.Matcher) bool {
	switch {
	case d.Name() == ".git":
		return true
	case r.MaxDepth > 0 && depth(rel) >= r.MaxDepth:
		return true
	case ig != nil && ig.Ignored(rel, true):
		return true
	}
	return r.Skip != nil && r.Skip(rel, true)
}

// keepFile reports whether file rel is visited.
func keepFile(rel string, d fs.DirEntry, r Rules, ig *ignore.Matcher) bool {
	t := d.Type()
	if t&fs.ModeSymlink != 0 {
		if !r.IncludeSymlinks {
			return false
		}
	} else if !t.IsRegular() {
		return false
	}
	if ig != nil && ig.Ignored(rel, false) {
		return false
	}
	return r.Skip == nil || !r.Skip(rel, false)
}

// depth counts the segments of a slash path ("a" = 1, "a/b" = 2).
func depth(rel string) int { return strings.Count(rel, "/") + 1 }
