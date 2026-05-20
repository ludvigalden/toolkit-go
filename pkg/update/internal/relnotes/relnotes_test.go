package relnotes

import (
	"strings"
	"testing"
)

func TestWrapAndIndent(t *testing.T) {
	got := indent(wrap("aaa bbb ccc", 7), "  ")
	if got != "  aaa bbb\n  ccc" {
		t.Errorf("got %q", got)
	}
}

func TestWritePublishedHidesVersionTitle(t *testing.T) {
	var b strings.Builder
	WritePublished(&b, "tool", "1.2.0", "2026-01-02", "v1.2.0", "fix\r\n")
	if b.String() != "tool 1.2.0 (2026-01-02) [published]\n  fix\n" {
		t.Errorf("got %q", b.String())
	}
}
