package sockpath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFallsBackForDeepDirs(t *testing.T) {
	base, err := os.MkdirTemp("/tmp", "sp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	deep := filepath.Join(base, strings.Repeat("d", 120))
	fb := filepath.Join(base, "fb")
	p, err := Resolve(deep, fb, "id")
	if err != nil || filepath.Dir(p.Socket) != fb {
		t.Fatalf("Resolve = %+v %v", p, err)
	}
	if info, err := os.Stat(fb); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("fallback mode: %v %v", info, err)
	}
	if p.Lock != p.Socket+LockSuffix || p.Temp != p.Socket+TempSuffix {
		t.Fatalf("neighbourhood = %+v", p)
	}
	if Hash("a") == Hash("b") || len(Hash("a")) != 64 {
		t.Fatal("hash not distinct hex sha256")
	}
}

func TestEnsurePrivateDirRejectsOpenDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if EnsurePrivateDir(dir) == nil {
		t.Fatal("group-readable dir accepted")
	}
}
