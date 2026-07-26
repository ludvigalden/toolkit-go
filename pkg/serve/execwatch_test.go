package serve

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchSignalsOnReplace(t *testing.T) {
	var mtime atomic.Int64
	mtime.Store(time.Now().Add(-time.Hour).UnixNano())
	stat := func() (time.Time, error) { return time.Unix(0, mtime.Load()), nil }
	ch, err := watch(context.Background(), 5*time.Millisecond, stat)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
		t.Fatal("signaled without a change")
	case <-time.After(30 * time.Millisecond):
	}
	mtime.Store(time.Now().UnixNano())
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before signaling")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no signal after the binary changed")
	}
	// The channel closes (no second signal) after signaling.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("signaled twice")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after signaling")
	}
}

func TestWatchStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := watch(ctx, time.Hour, func() (time.Time, error) { return time.Now(), nil })
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("signaled on cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close after cancel")
	}
}

func TestWatchStartupStatErrorIsError(t *testing.T) {
	// A watch that cannot start is an error: a closed channel would
	// terminate a healthy server with a nil return.
	boom := errors.New("gone")
	ch, err := watch(context.Background(), time.Hour, func() (time.Time, error) { return time.Time{}, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("startup stat error = %v, want wrapped boom", err)
	}
	if ch != nil {
		t.Fatal("startup stat error returned a channel")
	}
}

func TestWatchMidRunStatErrorIsChange(t *testing.T) {
	// A stat failure that begins mid-run is treated as a change: the
	// binary was replaced or removed under us.
	calls := 0
	stat := func() (time.Time, error) {
		calls++
		if calls == 1 {
			return time.Now(), nil
		}
		return time.Time{}, errors.New("gone")
	}
	ch, err := watch(context.Background(), 5*time.Millisecond, stat)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("channel closed instead of signaling the change")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mid-run stat failure did not signal")
	}
}

func TestWatchExecRealBinary(t *testing.T) {
	// Real os.Executable path: a poll over an unchanged binary must not
	// signal within a few intervals.
	ch, err := WatchExec(context.Background(), 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("signaled without a change")
		}
	case <-time.After(40 * time.Millisecond):
	}
}
