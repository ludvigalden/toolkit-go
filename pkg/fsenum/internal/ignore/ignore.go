// Package ignore is a minimal gitignore matcher for the WalkDir fallback:
// per-directory .gitignore and .ignore files, negation, anchoring,
// dir-only rules, and ** globs. Global and info/exclude files are not read.
package ignore

import (
	"bufio"
	"bytes"
	"io/fs"
	"path"
	"strings"
)

// fileNames are the ignore files read in every walked directory.
var fileNames = []string{".gitignore", ".ignore"}

// rule is one parsed pattern line.
type rule struct {
	segs     []string
	negate   bool
	dirOnly  bool
	anchored bool
}

// Matcher holds the rules of every directory loaded so far, keyed by the
// directory's slash path ("" for the root).
type Matcher struct {
	rules map[string][]rule
}

// New returns an empty Matcher.
func New() *Matcher { return &Matcher{rules: map[string][]rule{}} }

// Load reads the ignore files of dir (a slash path within fsys, "." or ""
// for the root). Missing or unreadable files contribute nothing.
func (m *Matcher) Load(fsys fs.FS, dir string) {
	if dir == "." {
		dir = ""
	}
	for _, name := range fileNames {
		data, err := fs.ReadFile(fsys, path.Join(orDot(dir), name))
		if err == nil {
			m.rules[dir] = append(m.rules[dir], parse(data)...)
		}
	}
}

// Ignored reports whether rel (a slash path) is ignored: the last matching
// rule of the nearest-to-deepest ancestor directories wins.
func (m *Matcher) Ignored(rel string, isDir bool) bool {
	ignored := false
	base := ""
	for {
		if rs := m.rules[base]; len(rs) > 0 {
			sub := strings.TrimPrefix(rel, base)
			sub = strings.TrimPrefix(sub, "/")
			for _, r := range rs {
				if r.matches(sub, isDir) {
					ignored = !r.negate
				}
			}
		}
		next, ok := nextAncestor(rel, base)
		if !ok {
			return ignored
		}
		base = next
	}
}

// parse parses gitignore-format content.
func parse(data []byte) []rule {
	var out []rule
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if r, ok := parseLine(sc.Text()); ok {
			out = append(out, r)
		}
	}
	return out
}

// parseLine parses one pattern line; ok is false for blanks and comments.
func parseLine(line string) (rule, bool) {
	line = strings.TrimRight(line, " \t\r")
	if line == "" || strings.HasPrefix(line, "#") {
		return rule{}, false
	}
	var r rule
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	} else if strings.HasPrefix(line, `\`) {
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimRight(line, "/")
	}
	if strings.Contains(line, "/") {
		r.anchored = true
		line = strings.TrimPrefix(line, "/")
	}
	if line == "" {
		return rule{}, false
	}
	r.segs = strings.Split(line, "/")
	return r, true
}

// matches tests sub, the path relative to the rule's directory.
func (r rule) matches(sub string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	if !r.anchored {
		ok, _ := path.Match(r.segs[0], path.Base(sub)) //nolint:errcheck // a malformed pattern never matches
		return ok
	}
	return matchSegs(r.segs, strings.Split(sub, "/"))
}

// matchSegs matches path segments against pattern segments, where "**"
// spans zero or more segments.
func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok { //nolint:errcheck // a malformed pattern never matches
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

// nextAncestor returns the next directory below base on the way to rel,
// excluding rel itself.
func nextAncestor(rel, base string) (string, bool) {
	rest := strings.TrimPrefix(strings.TrimPrefix(rel, base), "/")
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return "", false
	}
	if base == "" {
		return rest[:i], true
	}
	return base + "/" + rest[:i], true
}

func orDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}
