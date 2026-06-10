package gsux

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecutableResolvesSymlinks(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "bin")
	if err := os.WriteFile(real, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got, err := Executable(link); err != nil || got != real {
		t.Errorf("got %q err %v", got, err)
	}
	if got, err := Executable(""); err != nil || got == "" {
		t.Errorf("running binary: %q %v", got, err)
	}
}
