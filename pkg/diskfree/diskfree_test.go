package diskfree

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFreeExistingAndMissing(t *testing.T) {
	dir := t.TempDir()
	n, err := Free(dir)
	if err != nil || n == 0 {
		t.Fatalf("Free(dir) = %d, %v", n, err)
	}
	m, err := Free(filepath.Join(dir, "not-yet"))
	if err != nil || m == 0 {
		t.Fatalf("Free(missing child) = %d, %v", m, err)
	}
	if _, err := Free(filepath.Join(dir, "a", "b")); err == nil {
		t.Fatal("Free with a missing parent succeeded")
	}
}

func TestCheck(t *testing.T) {
	if err := Check(t.TempDir(), 1); err != nil {
		t.Fatalf("Check(1 byte) = %v", err)
	}
	stat := func(string) (uint64, error) { return 100, nil }
	if err := check(stat, "/x", 100); err != nil {
		t.Fatalf("check at the boundary = %v", err)
	}
	err := check(stat, "/x", 101)
	var low *LowDiskError
	if !errors.As(err, &low) || low.Free != 100 || low.Need != 101 || low.Path != "/x" {
		t.Fatalf("check below reserve = %v", err)
	}
	if low.Error() == "" {
		t.Fatal("empty error text")
	}
	boom := errors.New("boom")
	if err := check(func(string) (uint64, error) { return 0, boom }, "/x", 1); !errors.Is(err, boom) {
		t.Fatalf("stat failure = %v, want boom", err)
	}
}
