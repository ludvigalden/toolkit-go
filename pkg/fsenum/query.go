package fsenum

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/fsenum/internal/fdquery"
)

// ChangedSince lists files under root modified after since, as sorted
// slash paths relative to root.
func ChangedSince(ctx context.Context, root string, since time.Time, opts Options) ([]string, error) {
	args := []string{"--newer", since.Format(time.RFC3339Nano)}
	if paths, err := runFd(ctx, root, opts, args); err == nil || ctx.Err() != nil {
		return paths, err
	}
	var out []string
	err := walk(ctx, os.DirFS(root), opts, func(rel string, d fs.DirEntry) error {
		if info, ierr := d.Info(); ierr == nil && info.ModTime().After(since) {
			out = append(out, rel)
		}
		return nil
	})
	slices.Sort(out)
	return out, wrap(err)
}

// FindMarkers lists files named marker (an exact base name) under root, as
// sorted slash paths relative to root. Use path.Dir for the directory a
// marker marks.
func FindMarkers(ctx context.Context, root, marker string, opts Options) ([]string, error) {
	args := []string{"--glob", fdquery.GlobEscape(marker)}
	if paths, err := runFd(ctx, root, opts, args); err == nil || ctx.Err() != nil {
		return paths, err
	}
	var out []string
	err := walk(ctx, os.DirFS(root), opts, func(rel string, d fs.DirEntry) error {
		if d.Name() == marker {
			out = append(out, rel)
		}
		return nil
	})
	slices.Sort(out)
	return out, wrap(err)
}

// runFd runs the fd-backed query; any failure lets the caller fall back.
func runFd(ctx context.Context, root string, opts Options, extra []string) ([]string, error) {
	q := fdquery.Query{Skip: opts.Skip, NoIgnore: opts.NoIgnore, MaxDepth: opts.MaxDepth, Extra: extra}
	return fdquery.Run(ctx, opts.Runner, root, q) //nolint:wrapcheck // callers fall back
}

// wrap prefixes a fallback walk failure.
func wrap(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("fsenum: walk: %w", err)
}
