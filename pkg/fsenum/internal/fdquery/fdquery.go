// Package fdquery runs fd (or Debian's fdfind) for fsenum's queries and
// post-filters its NUL-separated output through a Go Skip callback.
package fdquery

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/procrun"
)

// Query is one fd invocation's pruning rules plus query-specific args.
type Query struct {
	Skip     func(rel string, isDir bool) bool
	NoIgnore bool
	MaxDepth int
	Extra    []string
}

// Run lists regular files under root as sorted slash paths. Any failure,
// fd missing included, lets the caller fall back to a walk.
func Run(ctx context.Context, rc *procrun.RunContext, root string, q Query) ([]string, error) {
	bin, err := rc.FindWithoutShell("fd")
	if err != nil {
		if bin, err = rc.FindWithoutShell("fdfind"); err != nil {
			return nil, err //nolint:wrapcheck // callers fall back
		}
	}
	res, err := rc.CmdPath(bin, Args(q)...).Dir(root).Run(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // callers fall back
	}
	return FilterSkip(SplitNull(res.Stdout), q.Skip), nil
}

// Args builds fd's argument list for q.
func Args(q Query) []string {
	args := []string{"-0", "--type", "f", "--hidden", "--no-require-git", "--exclude", ".git"}
	if q.NoIgnore {
		args = append(args, "--no-ignore")
	}
	if q.MaxDepth > 0 {
		args = append(args, "--max-depth", strconv.Itoa(q.MaxDepth))
	}
	return append(args, q.Extra...)
}

// FilterSkip drops paths skip rejects, directly or through a skipped
// ancestor directory (fd cannot prune with a Go callback), and sorts.
func FilterSkip(paths []string, skip func(string, bool) bool) []string {
	if skip == nil {
		slices.Sort(paths)
		return paths
	}
	dirs := map[string]bool{}
	skippedDir := func(dir string) bool {
		v, ok := dirs[dir]
		if !ok {
			v = skip(dir, true)
			dirs[dir] = v
		}
		return v
	}
	out := paths[:0]
next:
	for _, p := range paths {
		for i := 0; i < len(p); i++ {
			if p[i] == '/' && skippedDir(p[:i]) {
				continue next
			}
		}
		if !skip(p, false) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// SplitNull parses fd's -0 output into clean slash paths.
func SplitNull(b []byte) []string {
	var out []string
	for part := range bytes.SplitSeq(b, []byte{0}) {
		if len(part) == 0 {
			continue
		}
		out = append(out, path.Clean(strings.TrimPrefix(string(part), "./")))
	}
	return out
}

// GlobEscape makes name match literally as an fd glob.
func GlobEscape(name string) string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(`*?[]{}\`, r) {
			b.WriteByte('[')
			b.WriteRune(r)
			b.WriteByte(']')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
