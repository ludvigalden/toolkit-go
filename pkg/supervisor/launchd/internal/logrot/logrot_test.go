package logrot

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return "<absent>"
	}
	return string(b)
}

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "svc.log")
	write(t, live, "live-over-threshold")
	write(t, live+".1", "one")
	write(t, live+".2", "two")
	write(t, live+".9", "stray")
	if err := Rotate(live, 4, 2); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		live:        "<absent>",
		live + ".1": "live-over-threshold",
		live + ".2": "one",
		live + ".9": "<absent>",
	} {
		if got := read(t, path); got != want {
			t.Errorf("%s = %q, want %q", filepath.Base(path), got, want)
		}
	}
}

func TestRotateBelowThresholdOrAbsent(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "svc.log")
	if err := Rotate(live, 4, 3); err != nil {
		t.Fatalf("absent: %v", err)
	}
	write(t, live, "abc")
	if err := Rotate(live, 4, 3); err != nil || read(t, live) != "abc" {
		t.Fatalf("below threshold rotated: %v", err)
	}
}
