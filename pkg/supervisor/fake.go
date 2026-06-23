package supervisor

import (
	"context"
	"slices"
	"sync"
)

// Call records one Supervisor method invocation against a Fake.
type Call struct {
	Method string
	Spec   Spec
}

// Fake is an in-memory Supervisor that records calls and returns the
// configured results. Set the exported fields before use.
type Fake struct {
	// StatusReturn is what Status returns.
	StatusReturn Status
	// Err is returned by every method unless a more specific error is set.
	Err error
	// StartErr and StopErr override Err for Start and Stop.
	StartErr, StopErr error

	mu    sync.Mutex
	calls []Call
}

// NewFake returns an empty Fake.
func NewFake() *Fake { return &Fake{} }

var _ Supervisor = (*Fake)(nil)

// Calls returns a copy of the recorded calls in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Install records the call.
func (f *Fake) Install(_ context.Context, spec Spec) error {
	f.record("Install", spec)
	return f.Err
}

// Start records the call.
func (f *Fake) Start(_ context.Context, spec Spec) error {
	f.record("Start", spec)
	if f.StartErr != nil {
		return f.StartErr
	}
	return f.Err
}

// Stop records the call.
func (f *Fake) Stop(_ context.Context, spec Spec) error {
	f.record("Stop", spec)
	if f.StopErr != nil {
		return f.StopErr
	}
	return f.Err
}

// Status records the call.
func (f *Fake) Status(_ context.Context, spec Spec) (Status, error) {
	f.record("Status", spec)
	return f.StatusReturn, f.Err
}

// Uninstall records the call.
func (f *Fake) Uninstall(_ context.Context, spec Spec) error {
	f.record("Uninstall", spec)
	return f.Err
}

// record snapshots spec, copying Args so later caller mutation does not
// rewrite history.
func (f *Fake) record(method string, spec Spec) {
	spec.Args = slices.Clone(spec.Args)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Method: method, Spec: spec})
}

// FakeExecutor is a scripted CmdExecutor that records every command line.
type FakeExecutor struct {
	respond func(name string, args []string) (stdout, stderr []byte, err error)

	mu    sync.Mutex
	calls [][]string
}

// NewFakeExecutor returns a FakeExecutor answering with respond; nil
// answers every command with empty output and no error.
func NewFakeExecutor(respond func(name string, args []string) (stdout, stderr []byte, err error)) *FakeExecutor {
	return &FakeExecutor{respond: respond}
}

var _ CmdExecutor = (*FakeExecutor)(nil)

// Run records name and args, then answers via the respond func.
func (f *FakeExecutor) Run(_ context.Context, name string, args ...string) (stdout, stderr []byte, err error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{name}, args...))
	f.mu.Unlock()
	if f.respond == nil {
		return nil, nil, nil
	}
	return f.respond(name, args)
}

// Calls returns a copy of the recorded command lines, name first.
func (f *FakeExecutor) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = slices.Clone(c)
	}
	return out
}
