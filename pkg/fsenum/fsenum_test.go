package fsenum

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procrun"
)

// tree builds a fixture repository and returns its root.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".gitignore":          "*.log\nbuild/\n",
		"a.go":                "",
		"x.log":               "",
		".hidden":             "",
		".git/HEAD":           "",
		"build/out.bin":       "",
		"node_modules/m/i.js": "",
		"pkg/b.go":            "",
		"pkg/.ignore":         "skip.go\n",
		"pkg/skip.go":         "",
		"pkg/deep/app.marker": "",
		"app.marker":          "",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil { //nolint:gosec // fixture
			t.Fatal(err)
		}
	}
	return root
}

// modes runs fn once with fd available (when installed) and once forced
// onto the WalkDir fallback via an empty PATH.
func modes(t *testing.T, fn func(t *testing.T, opts Options)) {
	t.Helper()
	t.Run("fd", func(t *testing.T) {
		rc := procrun.NewRunContext("")
		if _, err := rc.FindWithoutShell("fd"); err != nil {
			if _, err := rc.FindWithoutShell("fdfind"); err != nil {
				t.Skip("fd not installed")
			}
		}
		fn(t, Options{Runner: rc})
	})
	t.Run("walk", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		fn(t, Options{Runner: procrun.NewRunContext("")})
	})
}

func TestWalkHonorsIgnoreAndSkip(t *testing.T) {
	root := tree(t)
	got := paths(t, root, Options{Skip: Set{Dirs: []string{"node_modules"}}.Skip()})
	want := []string{".gitignore", ".hidden", "a.go", "app.marker", "pkg/.ignore", "pkg/b.go", "pkg/deep/app.marker"}
	if !slices.Equal(got, want) {
		t.Fatalf("Walk = %v\nwant %v", got, want)
	}
	all := paths(t, root, Options{NoIgnore: true, MaxDepth: 1})
	if !slices.Equal(all, []string{".gitignore", ".hidden", "a.go", "app.marker", "x.log"}) {
		t.Fatalf("NoIgnore+MaxDepth = %v", all)
	}
	files := paths(t, root, Options{Skip: func(rel string, isDir bool) bool { return !isDir && filepath.Ext(rel) != ".go" }})
	if !slices.Equal(files, []string{"a.go", "pkg/b.go"}) {
		t.Fatalf("file Skip = %v", files)
	}
}

func TestWalkSymlinksAndStop(t *testing.T) {
	root := tree(t)
	if err := os.Symlink("a.go", filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(paths(t, root, Options{}), "link.go") {
		t.Fatal("symlink listed without IncludeSymlinks")
	}
	es, err := List(context.Background(), os.DirFS(root), Options{IncludeSymlinks: true, MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	var link *Entry
	for i := range es {
		if es[i].Path == "link.go" {
			link = &es[i]
		}
		if es[i].Path == "dangling" {
			t.Fatal("dangling symlink listed")
		}
	}
	if link == nil || !link.IsSymlink {
		t.Fatalf("link entry = %+v", link)
	}
	n := 0
	for range Walk(context.Background(), os.DirFS(root), Options{}) {
		n++
		break
	}
	if n != 1 {
		t.Fatal("early break did not stop")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := List(ctx, os.DirFS(root), Options{}); err == nil {
		t.Fatal("canceled walk succeeded")
	}
}

func TestChangedSince(t *testing.T) {
	modes(t, func(t *testing.T, opts Options) {
		root := tree(t)
		past := time.Now().Add(-time.Hour)
		for _, rel := range []string{".gitignore", ".hidden", "a.go", "x.log", "app.marker", "pkg/b.go", "pkg/.ignore", "pkg/skip.go", "pkg/deep/app.marker", "node_modules/m/i.js", "build/out.bin"} {
			if err := os.Chtimes(filepath.Join(root, rel), past, past); err != nil {
				t.Fatal(err)
			}
		}
		since := time.Now().Add(-time.Minute)
		for _, rel := range []string{"a.go", "x.log", "pkg/b.go", "node_modules/m/i.js"} {
			if err := os.Chtimes(filepath.Join(root, rel), time.Now(), time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		opts.Skip = Set{Dirs: []string{"node_modules"}}.Skip()
		got, err := ChangedSince(context.Background(), root, since, opts)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{"a.go", "pkg/b.go"}) {
			t.Fatalf("ChangedSince = %v", got)
		}
	})
}

func TestFindMarkers(t *testing.T) {
	modes(t, func(t *testing.T, opts Options) {
		root := tree(t)
		got, err := FindMarkers(context.Background(), root, "app.marker", opts)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{"app.marker", "pkg/deep/app.marker"}) {
			t.Fatalf("FindMarkers = %v", got)
		}
		opts.MaxDepth = 2
		got, _ = FindMarkers(context.Background(), root, "app.marker", opts)
		if !slices.Equal(got, []string{"app.marker"}) {
			t.Fatalf("FindMarkers depth 2 = %v", got)
		}
		opts.MaxDepth = 0
		opts.Skip = Set{Dirs: []string{"deep"}}.Skip()
		got, _ = FindMarkers(context.Background(), root, "app.marker", opts)
		if !slices.Equal(got, []string{"app.marker"}) {
			t.Fatalf("FindMarkers skip = %v", got)
		}
	})
}

func paths(t *testing.T, root string, opts Options) []string {
	t.Helper()
	es, err := List(context.Background(), os.DirFS(root), opts)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Path)
	}
	return out
}

func TestDefaultSkip(t *testing.T) {
	skip := Default().Skip()
	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{"node_modules", true, true},
		{"a/b/.venv", true, true},
		{"web/.next", true, true},
		{"src", true, false},
		{"build", true, false},
		{"tmp-123", true, true},
		{"_legacy-v1", true, true},
		{"a/.DS_Store", false, true},
		{"x/app.min.js", false, true},
		{"lib.so", false, true},
		{"data.sqlite3", false, true},
		{"notes.txt.bak", false, true},
		{"main.go", false, false},
		{"go.sum", false, false},
		{"package-lock.json", false, false},
		{".env", false, false},
		{"logo.png", false, false},
		{"api.pb.go", false, false},
	}
	for _, c := range cases {
		if got := skip(c.rel, c.isDir); got != c.want {
			t.Errorf("Default().Skip()(%q, dir=%v) = %v, want %v", c.rel, c.isDir, got, c.want)
		}
	}
}

func TestSetUnionCopies(t *testing.T) {
	base := VCS()
	u := base.Union(Set{Dirs: []string{"extra", ".git"}}, Images())
	if len(base.Dirs) != len(VCS().Dirs) {
		t.Fatalf("Union mutated its receiver")
	}
	skip := u.Skip()
	if !skip("extra", true) || !skip(".git", true) || !skip("a.png", false) {
		t.Fatalf("union lost entries: %+v", u)
	}
	n := 0
	for _, d := range u.Dirs {
		if d == ".git" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf(".git appears %d times, want 1", n)
	}
}
