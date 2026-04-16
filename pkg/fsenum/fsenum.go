package fsenum

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/fsenum/internal/walker"
	"github.com/ludvigalden/toolkit-go/pkg/procrun"
)

// Entry is the metadata of one enumerated file. Path is slash-separated
// and relative to the walk root. With Options.IncludeSymlinks, symlink
// entries carry lstat metadata: Size is the link's own length, not the
// target file's.
type Entry struct {
	Path      string
	Size      int64
	ModTime   time.Time
	Mode      fs.FileMode
	IsSymlink bool
}

// Options controls enumeration. The zero value lists every regular file,
// hidden ones included, honoring .gitignore and .ignore and pruning .git.
type Options struct {
	// Skip prunes a directory (isDir) or drops a file; rel is the
	// slash path relative to the root. Default().Skip() is the usual
	// choice; nil skips nothing beyond .git and ignore files.
	Skip func(rel string, isDir bool) bool
	// NoIgnore stops honoring .gitignore and .ignore files.
	NoIgnore bool
	// MaxDepth bounds descent: 1 lists the root's own files only; 0 is
	// unlimited.
	MaxDepth int
	// IncludeSymlinks makes Walk yield symlinks to files (fd-backed
	// queries always list regular files only).
	IncludeSymlinks bool
	// Runner runs fd; nil uses procrun's default context.
	Runner *procrun.RunContext
}

// Walk yields an Entry for every permitted file under fsys, reading no
// file contents. Vanishing paths are skipped; other errors end the walk
// with a final (Entry{}, err).
func Walk(ctx context.Context, fsys fs.FS, opts Options) iter.Seq2[Entry, error] {
	return func(yield func(Entry, error) bool) {
		err := walk(ctx, fsys, opts, func(rel string, d fs.DirEntry) error {
			info, err := d.Info()
			if err != nil {
				return nil //nolint:nilerr // vanished between read and stat
			}
			isLink := d.Type()&fs.ModeSymlink != 0
			if isLink {
				target, serr := fs.Stat(fsys, rel)
				if serr != nil || !target.Mode().IsRegular() {
					return nil //nolint:nilerr // dangling or non-file symlink
				}
			}
			e := Entry{Path: rel, Size: info.Size(), ModTime: info.ModTime(), Mode: info.Mode(), IsSymlink: isLink}
			if !yield(e, nil) {
				return errStop
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStop) {
			yield(Entry{}, fmt.Errorf("fsenum: walk: %w", err))
		}
	}
}

// List drains Walk into a slice, stopping at the first error.
func List(ctx context.Context, fsys fs.FS, opts Options) ([]Entry, error) {
	var out []Entry
	for e, err := range Walk(ctx, fsys, opts) {
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// errStop ends a walk early when the consumer stops iterating.
var errStop = errors.New("stop")

// walk runs the shared traversal with opts' pruning rules.
func walk(ctx context.Context, fsys fs.FS, opts Options, visit func(rel string, d fs.DirEntry) error) error {
	r := walker.Rules{Skip: opts.Skip, NoIgnore: opts.NoIgnore, MaxDepth: opts.MaxDepth, IncludeSymlinks: opts.IncludeSymlinks}
	return walker.Walk(ctx, fsys, r, visit) //nolint:wrapcheck // callers wrap
}
