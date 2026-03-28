package lockfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTryOpenRewriteFree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.lock")
	f, err := TryOpen(path)
	if err != nil || f == nil {
		t.Fatalf("TryOpen = %v %v", f, err)
	}
	if g, err := TryOpen(path); g != nil || err != nil {
		t.Fatalf("contended TryOpen = %v %v", g, err)
	}
	if Free(path) {
		t.Fatal("held lock reported free")
	}
	if err := Rewrite(f, []byte("body")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "body" {
		t.Fatalf("body = %q", b)
	}
	_ = f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitFree(ctx, path); err != nil {
		t.Fatal(err)
	}
}

func TestSweep(t *testing.T) {
	dir := t.TempDir()
	dead, live := filepath.Join(dir, "dead.lock"), filepath.Join(dir, "live.lock")
	for _, p := range []string{dead, live} {
		if err := os.WriteFile(p, []byte(filepath.Base(p)), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		_ = os.Chtimes(p, old, old)
	}
	Sweep(dir, "", time.Minute, func(b []byte) bool { return string(b) == "dead.lock" })
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Fatal("dead lock survived")
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("live lock removed")
	}
}
