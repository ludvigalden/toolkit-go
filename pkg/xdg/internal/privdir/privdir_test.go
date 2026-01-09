package privdir

import (
	"os"
	"testing"
)

func TestEnsure(t *testing.T) {
	base := t.TempDir()
	dir, err := Ensure(base, "app")
	if err != nil || dir != UserPath(base, "app") {
		t.Fatalf("Ensure = %q %v", dir, err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(base, "app"); err == nil {
		t.Fatal("open dir accepted")
	}
	for _, bad := range []string{"", ".", "..", "a/b"} {
		if CheckName(bad) == nil {
			t.Errorf("CheckName(%q) accepted", bad)
		}
	}
}
