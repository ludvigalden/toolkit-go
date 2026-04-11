package walker

import (
	"context"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"
)

func TestWalkPrunes(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":   {Data: []byte("*.log\n")},
		"a.go":         {},
		"x.log":        {},
		".git/HEAD":    {},
		"d/e/deep.go":  {},
		"skip/file.go": {},
	}
	var got []string
	err := Walk(context.Background(), fsys, Rules{
		MaxDepth: 2,
		Skip:     func(rel string, isDir bool) bool { return isDir && rel == "skip" },
	}, func(rel string, _ fs.DirEntry) error {
		got = append(got, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{".gitignore", "a.go"}) {
		t.Fatalf("visited = %v", got)
	}
}
