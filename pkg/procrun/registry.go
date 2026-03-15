package procrun

import (
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/track"
)

// Registry tracks live processes spawned through the RunContexts that
// carry it, so a binary can stop them in order on exit or signal.
type Registry struct {
	set *track.Set
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry { return &Registry{set: track.New()} }

func (r *Registry) register(p *Process, kind track.Kind) {
	if r != nil {
		r.set.Add(p.p.Cmd.Process, kind)
	}
}

func (r *Registry) deregister(p *Process) {
	if r != nil {
		r.set.Remove(p.p.Cmd.Process)
	}
}

// Len reports how many tracked processes have not been reaped yet.
func (r *Registry) Len() int {
	if r.set == nil {
		return 0
	}
	return r.set.Len()
}

// Cleanup stops every tracked process: long-running ones get SIGTERM and
// up to grace to exit, then everything left is killed. It is idempotent
// and safe to call concurrently.
func (r *Registry) Cleanup(grace time.Duration) {
	if r.set != nil {
		r.set.Cleanup(grace)
	}
}

// CleanupOnSignal runs Cleanup(grace) on SIGINT or SIGTERM until the
// returned stop is called. A second call while installed is a no-op.
func (r *Registry) CleanupOnSignal(grace time.Duration) (stop func()) {
	if r.set == nil {
		return func() {}
	}
	return r.set.CleanupOnSignal(grace)
}
