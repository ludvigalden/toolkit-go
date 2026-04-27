package lockfile

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLockAndRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	f, err := TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TryLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second TryLock = %v", err)
	}
	if !Held(path) {
		t.Fatal("Held = false while locked")
	}
	want := Record{Hash: "h", Socket: "/s", Inode: 7, PID: 9}
	if err := Write(f, want); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(f); err != nil || got != want {
		t.Fatalf("Read = %+v %v", got, err)
	}
	if got, err := ReadFile(path); err != nil || got != want {
		t.Fatalf("ReadFile = %+v %v", got, err)
	}
	_ = f.Close()
	if !WaitFree(context.Background(), path, time.Second) {
		t.Fatal("lock not free after close")
	}
}
