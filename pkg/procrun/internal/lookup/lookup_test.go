package lookup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSiblingAndCache(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "tool-"+runtime.GOARCH)
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test executable
		t.Fatal(err)
	}
	if p, ok := Sibling(dir, "tool"); !ok || p != want {
		t.Fatalf("Sibling = %q %v", p, ok)
	}
	if IsExecutable(dir) {
		t.Fatal("directory reported executable")
	}
	var c Cache
	if p, ok := Path(&c, "sh"); !ok || !filepath.IsAbs(p) {
		t.Fatalf("Path(sh) = %q %v", p, ok)
	}
	if _, ok := c.Get("sh"); !ok {
		t.Fatal("hit not cached")
	}
}
