// Package track keeps the set of live processes a registry owns and
// stops them in order: long-running ones gracefully, then the rest.
package track

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Kind orders processes for cleanup.
type Kind int

// Process kinds.
const (
	OneShot Kind = iota
	LongRunning
)

// Set is a concurrency-safe set of live processes. Use New.
type Set struct {
	mu      sync.Mutex
	m       map[*os.Process]Kind
	sigStop func()
}

// New returns an empty Set.
func New() *Set { return &Set{m: make(map[*os.Process]Kind)} }

// Add tracks p; a nil Set ignores it.
func (s *Set) Add(p *os.Process, kind Kind) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[p] = kind
}

// Remove forgets p; a nil Set ignores it.
func (s *Set) Remove(p *os.Process) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, p)
}

// Len reports how many processes are tracked.
func (s *Set) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// Cleanup empties the set and stops its processes: long-running ones get
// SIGTERM and up to grace to exit, then everything left is killed.
func (s *Set) Cleanup(grace time.Duration) {
	s.mu.Lock()
	var long, short []*os.Process
	for p, kind := range s.m {
		if kind == LongRunning {
			long = append(long, p)
		} else {
			short = append(short, p)
		}
	}
	s.m = make(map[*os.Process]Kind)
	s.mu.Unlock()

	for _, p := range long {
		_ = p.Signal(syscall.SIGTERM) //nolint:errcheck // already-exited processes are fine
	}
	if len(long) > 0 && grace > 0 {
		deadline := time.Now().Add(grace)
		for time.Now().Before(deadline) && !allExited(long) {
			time.Sleep(20 * time.Millisecond)
		}
	}
	for _, p := range append(long, short...) {
		_ = p.Kill() //nolint:errcheck // already-exited processes are fine
	}
}

// CleanupOnSignal runs Cleanup(grace) on SIGINT or SIGTERM until stop is
// called. A second call while installed is a no-op.
func (s *Set) CleanupOnSignal(grace time.Duration) (stop func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sigStop != nil {
		return func() {}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				s.Cleanup(grace)
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	s.sigStop = func() {
		once.Do(func() {
			signal.Stop(ch)
			close(done)
		})
	}
	return func() {
		s.mu.Lock()
		f := s.sigStop
		s.sigStop = nil
		s.mu.Unlock()
		if f != nil {
			f()
		}
	}
}

// allExited probes liveness with signal 0, which fails once a process is
// reaped; it avoids racing the owner's Wait on ProcessState.
func allExited(ps []*os.Process) bool {
	for _, p := range ps {
		if p.Signal(syscall.Signal(0)) == nil {
			return false
		}
	}
	return true
}
