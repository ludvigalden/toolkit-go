package serve

import (
	"context"
	"fmt"
	"os"
	"time"
)

// DefaultExecWatchInterval is the default mtime poll for [WatchExec].
const DefaultExecWatchInterval = 2 * time.Second

// WatchExec signals once on the returned channel when the running
// executable's mtime changes or it stops being statable mid-run (a
// rebuild replaced or removed it). A watch that cannot start returns an
// error rather than a channel that would read as a change.
func WatchExec(ctx context.Context, interval time.Duration) (<-chan struct{}, error) {
	return watch(ctx, interval, statExec)
}

// watch is WatchExec with the stat seam injected for tests.
func watch(ctx context.Context, interval time.Duration, stat func() (time.Time, error)) (<-chan struct{}, error) {
	initial, err := stat()
	if err != nil {
		// Watcher death at startup is surfaced, not hidden: a closed
		// channel would terminate a healthy server with a nil error.
		return nil, fmt.Errorf("serve: watch executable: %w", err)
	}
	ch := make(chan struct{}, 1)
	if interval <= 0 {
		interval = DefaultExecWatchInterval
	}
	go func() {
		defer close(ch)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current, err := stat()
				if err != nil {
					// Mid-run stat failure means the binary was replaced
					// or removed under us: a change, not a watcher death.
					current = initial.Add(1)
				}
				if !current.Equal(initial) {
					// Buffer 1: the signal is dropped, not blocking, when
					// nobody reads before close.
					select {
					case ch <- struct{}{}:
					default:
					}
					return
				}
			}
		}
	}()
	return ch, nil
}

func statExec() (time.Time, error) {
	exe, err := os.Executable()
	if err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(exe)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
