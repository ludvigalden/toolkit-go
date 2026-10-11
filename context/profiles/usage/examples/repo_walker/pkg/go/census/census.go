// Package census counts a repository's files by extension, honoring
// .gitignore and skipping fsenum's default dependency, build, cache and
// binary sets plus the directories a caller names.
package census

import (
	"context"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/fsenum"
)

// CacheDir is the tool's own runtime directory inside a repository; the
// walk always prunes it so the tool never counts its own state.
const CacheDir = ".census"

// Options narrows a census.
type Options struct {
	// SkipDirs are extra directory names pruned at any depth, on top of
	// fsenum.Default() and the tool's own CacheDir.
	SkipDirs []string
	// NoIgnore counts gitignored files too.
	NoIgnore bool
}

// Report is the census result.
type Report struct {
	Files      int            `json:"files"`
	Bytes      int64          `json:"bytes"`
	Extensions map[string]int `json:"extensions"`
}

// Take walks fsys and counts each file under its lowercased extension
// ("(none)" for extensionless files).
func Take(ctx context.Context, fsys fs.FS, opts Options) (Report, error) {
	skip := fsenum.Default().Union(fsenum.Set{Dirs: append([]string{CacheDir}, opts.SkipDirs...)}).Skip()
	r := Report{Extensions: map[string]int{}}
	for e, err := range fsenum.Walk(ctx, fsys, fsenum.Options{Skip: skip, NoIgnore: opts.NoIgnore}) {
		if err != nil {
			return r, err
		}
		r.Files++
		r.Bytes += e.Size
		ext := strings.ToLower(path.Ext(e.Path))
		if ext == "" {
			ext = "(none)"
		}
		r.Extensions[ext]++
	}
	return r, nil
}

// Sorted returns m's keys by descending count, then name.
func Sorted(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if m[a] != m[b] {
			return m[b] - m[a]
		}
		return strings.Compare(a, b)
	})
	return keys
}
