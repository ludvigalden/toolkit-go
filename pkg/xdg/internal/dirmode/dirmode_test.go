package dirmode

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRepair(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	if got, err := Ensure(dir, 0o700); err != nil || got != 0o700 {
		t.Fatalf("create = %o %v", got, err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, _ := Ensure(dir, 0o700); got != 0o755 {
		t.Fatalf("existing = %o", got)
	}
	if err := Repair(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "f")
	_ = os.WriteFile(f, nil, 0o600)
	if _, err := Ensure(f, 0o700); !errors.Is(err, ErrNotDir) {
		t.Fatalf("file = %v", err)
	}
}
