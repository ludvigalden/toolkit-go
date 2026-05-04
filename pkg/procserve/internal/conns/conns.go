// Package conns counts a server's in-flight handlers, tracks when it
// went idle, and gates admission once draining starts.
package conns

import (
	"sync"
	"time"
)

// Tracker is one server's connection bookkeeping. Use New.
type Tracker struct {
	mu        sync.Mutex
	inFlight  int
	idleSince time.Time
	draining  bool
	handlers  sync.WaitGroup
	changed   chan struct{}
}

// New returns a Tracker idle since now.
func New(now time.Time) *Tracker {
	return &Tracker{idleSince: now, changed: make(chan struct{}, 1)}
}

// Changed fires after the in-flight count changed.
func (t *Tracker) Changed() <-chan struct{} { return t.changed }

// Admit counts a new handler unless draining; the check and the
// WaitGroup Add share the lock so Drain never misses a handler.
func (t *Tracker) Admit() bool {
	t.mu.Lock()
	if t.draining {
		t.mu.Unlock()
		return false
	}
	t.handlers.Add(1)
	t.inFlight++
	t.mu.Unlock()
	t.notify()
	return true
}

// Done ends one admitted handler.
func (t *Tracker) Done() {
	t.mu.Lock()
	if t.inFlight--; t.inFlight == 0 {
		t.idleSince = time.Now()
	}
	t.mu.Unlock()
	t.handlers.Done()
	t.notify()
}

// Idle reports that no handler is running.
func (t *Tracker) Idle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inFlight == 0
}

// Snapshot returns the current counts.
func (t *Tracker) Snapshot() (inFlight int, idleSince time.Time, draining bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inFlight, t.idleSince, t.draining
}

// StartDrain refuses further admissions.
func (t *Tracker) StartDrain() {
	t.mu.Lock()
	t.draining = true
	t.mu.Unlock()
}

// Wait blocks until every admitted handler is done; after grace it calls
// cancel and keeps waiting.
func (t *Tracker) Wait(grace time.Duration, cancel func()) {
	done := make(chan struct{})
	go func() { t.handlers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(grace):
		cancel()
		<-done
	}
}

func (t *Tracker) notify() {
	select {
	case t.changed <- struct{}{}:
	default:
	}
}

// Idler is one idle server as LRU eviction sees it.
type Idler struct {
	Key       string
	IdleSince time.Time
}

// LeastRecent returns the index of the longest-idle entry (ties broken
// by Key), or -1 when fewer than limit entries are idle.
func LeastRecent(idle []Idler, limit int) int {
	if limit <= 0 || len(idle) < limit {
		return -1
	}
	best := 0
	for i, c := range idle[1:] {
		b := idle[best]
		if d := c.IdleSince.Compare(b.IdleSince); d < 0 || (d == 0 && c.Key < b.Key) {
			best = i + 1
		}
	}
	return best
}
