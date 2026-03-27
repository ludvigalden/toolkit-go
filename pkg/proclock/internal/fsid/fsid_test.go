package fsid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSegment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(home, "Code", "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	realOutside, _ := filepath.EvalSymlinks(outside)
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"home", home, "~"},
		{"under home", repo, "~/Code/repo"},
		{"under home unclean", filepath.Join(repo, "..", "repo"), "~/Code/repo"},
		{"outside home", outside, realOutside},
		{"missing path stays verbatim", filepath.Join(realHome, "nope"), "~/nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Segment(tt.in)
			if err != nil || got != tt.want {
				t.Errorf("Segment(%s) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}
