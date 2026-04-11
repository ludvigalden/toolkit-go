package ignore

import (
	"testing"
	"testing/fstest"
)

func TestMatcher(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":     {Data: []byte("# comment\n*.log\n!keep.log\nbuild/\n/top.txt\ndocs/**/gen\n\\#hash\n")},
		"sub/.gitignore": {Data: []byte("local.txt\n/anch.txt\n")},
		"sub/.ignore":    {Data: []byte("!re.log\n")},
	}
	m := New()
	m.Load(fsys, ".")
	m.Load(fsys, "sub")
	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{"a.log", false, true},
		{"deep/x/a.log", false, true},
		{"keep.log", false, false},
		{"build", true, true},
		{"build", false, false},
		{"top.txt", false, true},
		{"x/top.txt", false, false},
		{"docs/gen", true, true},
		{"docs/a/b/gen", true, true},
		{"#hash", false, true},
		{"sub/local.txt", false, true},
		{"sub/deeper/local.txt", false, true},
		{"local.txt", false, false},
		{"sub/anch.txt", false, true},
		{"sub/x/anch.txt", false, false},
		{"sub/re.log", false, false},
		{"plain.go", false, false},
	}
	for _, c := range cases {
		if got := m.Ignored(c.rel, c.isDir); got != c.want {
			t.Errorf("Ignored(%q, %v) = %v, want %v", c.rel, c.isDir, got, c.want)
		}
	}
}
