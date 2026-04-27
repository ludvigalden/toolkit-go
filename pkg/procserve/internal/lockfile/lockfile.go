// Package lockfile is the identity lock: a never-unlinked file under an
// exclusive flock whose JSON record tells the server its socket and the
// inode it may unlink.
package lockfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Record is the lock file's content.
type Record struct {
	Hash   string `json:"hash"`
	Socket string `json:"socket"`
	Inode  uint64 `json:"inode"`
	PID    int    `json:"pid,omitempty"`
}

// ErrLocked reports a lock held by another live process.
var ErrLocked = errors.New("procserve: lock held")

// TryLock opens (creating) path and takes a non-blocking exclusive flock.
// The lock file is never unlinked, so every racer contends on one inode.
func TryLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("procserve: lock %s: %w", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close() //nolint:errcheck // probe descriptor
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("procserve: flock %s: %w", path, err)
	}
	return f, nil
}

// Held reports whether some process holds the lock at path.
func Held(path string) bool {
	f, err := TryLock(path)
	if err == nil {
		_ = f.Close() //nolint:errcheck // probe only
		return false
	}
	return errors.Is(err, ErrLocked)
}

// Write replaces the lock file's content (the holder is exclusive).
func Write(f *os.File, r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("procserve: lock record: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("procserve: lock record: %w", err)
	}
	if _, err := f.WriteAt(b, 0); err != nil {
		return fmt.Errorf("procserve: lock record: %w", err)
	}
	return nil
}

// Read reads the record through r from offset 0.
func Read(r io.ReaderAt) (Record, error) {
	b, err := io.ReadAll(io.NewSectionReader(r, 0, 1<<20))
	if err != nil {
		return Record{}, fmt.Errorf("procserve: lock record: %w", err)
	}
	return decode(b)
}

// ReadFile reads a lock file's record by path.
func ReadFile(path string) (Record, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("procserve: lock record: %w", err)
	}
	return decode(b)
}

func decode(b []byte) (Record, error) {
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil {
		return Record{}, fmt.Errorf("procserve: lock record: %w", err)
	}
	return rec, nil
}

// WaitFree polls until no process holds the lock or bound elapses.
func WaitFree(ctx context.Context, path string, bound time.Duration) bool {
	deadline := time.Now().Add(bound)
	for Held(path) {
		if time.Now().After(deadline) || Sleep(ctx, 20*time.Millisecond) != nil {
			return false
		}
	}
	return true
}

// Sleep waits d or until ctx ends.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // context errors pass through
	case <-t.C:
		return nil
	}
}
