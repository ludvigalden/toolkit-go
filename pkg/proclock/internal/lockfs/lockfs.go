// Package lockfs is the file side of a flock-based lock: non-blocking
// acquisition that survives a racing unlink, durable body rewrites,
// free probes, and the sweep of orphaned lock files.
package lockfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/proclock/internal/osflock"
)

// TryOpen opens (creating) path and takes a non-blocking exclusive flock.
// It returns (nil, nil) when contended or when a racing release unlinked
// the path between open and lock.
func TryOpen(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("proclock: open %s: %w", path, err)
	}
	if lerr := osflock.TryLockEx(f.Fd()); lerr != nil {
		_ = f.Close() //nolint:errcheck // probe descriptor
		if osflock.Contended(lerr) {
			return nil, nil
		}
		return nil, fmt.Errorf("proclock: flock %s: %w", path, lerr)
	}
	if st, serr := os.Stat(path); serr != nil || !sameFile(f, st) {
		_ = f.Close() //nolint:errcheck // stale inode
		return nil, nil
	}
	return f, nil
}

// sameFile reports whether the open file is the one now at its path.
func sameFile(f *os.File, st os.FileInfo) bool {
	fst, err := f.Stat()
	return err == nil && os.SameFile(fst, st)
}

// Rewrite replaces the file's content with b and fsyncs it.
func Rewrite(f *os.File, b []byte) error {
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("proclock: truncate: %w", err)
	}
	if _, err := f.WriteAt(b, 0); err != nil {
		return fmt.Errorf("proclock: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("proclock: sync: %w", err)
	}
	return nil
}

// Free probes for an exclusive holder with a shared non-blocking flock;
// an absent file is free.
func Free(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	defer f.Close() //nolint:errcheck // probe descriptor
	return osflock.TryLockSh(f.Fd()) == nil
}

// WaitFree polls with backoff (50ms doubling to 1s) until Free.
func WaitFree(ctx context.Context, path string) error {
	d := 50 * time.Millisecond
	for !Free(path) {
		if err := Sleep(ctx, d); err != nil {
			return err
		}
		d = min(2*d, time.Second)
	}
	return nil
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

// Sweep removes *.lock files in dir other than own that are older than
// staleAge, unheld, non-empty, and judged dead by the body. Every probe
// is non-blocking.
func Sweep(dir, own string, staleAge time.Duration, dead func(body []byte) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleAge)
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() || p == own || !strings.HasSuffix(e.Name(), ".lock") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			removeIfDead(p, dead)
		}
	}
}

// removeIfDead unlinks path when no one holds it and dead accepts its
// body; empty files (mid-creation) are left alone.
func removeIfDead(path string, dead func([]byte) bool) {
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck // probe descriptor
	if osflock.TryLockEx(f.Fd()) != nil {
		return
	}
	raw, err := io.ReadAll(f)
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 || !dead(raw) {
		return
	}
	_ = os.Remove(path) //nolint:errcheck // best-effort
}
