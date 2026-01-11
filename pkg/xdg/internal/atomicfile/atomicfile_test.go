package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "f", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "f"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v %v", info, err)
	}
	if Write(dir, "a/b", nil, 0o600) == nil {
		t.Fatal("non-bare name accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temp files left: %v", entries)
	}
}
