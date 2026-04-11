package fdquery

import (
	"slices"
	"testing"
)

func TestGlobEscape(t *testing.T) {
	if got := GlobEscape("a*b?.[x]"); got != "a[*]b[?].[[]x[]]" {
		t.Fatalf("GlobEscape = %q", got)
	}
}

func TestSplitAndFilter(t *testing.T) {
	paths := SplitNull([]byte("./b.go\x00skip/x.go\x00a.go\x00"))
	got := FilterSkip(paths, func(rel string, isDir bool) bool { return isDir && rel == "skip" })
	if !slices.Equal(got, []string{"a.go", "b.go"}) {
		t.Fatalf("filtered = %v", got)
	}
	if args := Args(Query{NoIgnore: true, MaxDepth: 2, Extra: []string{"--newer", "x"}}); !slices.Contains(args, "--no-ignore") || args[len(args)-1] != "x" {
		t.Fatalf("Args = %v", args)
	}
}
