//go:build linux

package selector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/proclock"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

func probe(linger, state string) *supervisor.FakeExecutor {
	return supervisor.NewFakeExecutor(func(name string, _ []string) ([]byte, []byte, error) {
		if name == "loginctl" {
			return []byte("Linger=" + linger + "\n"), nil, nil
		}
		return []byte(state + "\n"), nil, nil
	})
}

func opts(t *testing.T) Options {
	home := t.TempDir()
	return Options{LockDir: t.TempDir(), Getenv: func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}}
}

func TestSelectLinux(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		linger, state string
		want          Mode
	}{
		{"yes", "running", ModeSystemd},
		{"yes", "degraded", ModeSystemd},
		{"no", "running", ModeSetsid},
		{"yes", "offline", ModeSetsid},
	} {
		sel, err := New(ctx, probe(tc.linger, tc.state), opts(t))
		if err != nil {
			t.Fatal(err)
		}
		if sel.Mode() != tc.want {
			t.Errorf("linger=%s state=%s: mode %s, want %s", tc.linger, tc.state, sel.Mode(), tc.want)
		}
		line := sel.DiagnosticLine()
		if !strings.HasPrefix(line, "supervisor: "+string(tc.want)+" (selected because: ") || !strings.HasSuffix(line, "\n") {
			t.Errorf("DiagnosticLine = %q", line)
		}
	}
}

func TestNewValidates(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, nil, opts(t)); err == nil {
		t.Error("nil executor accepted")
	}
	if _, err := New(ctx, probe("yes", "running"), Options{}); err == nil {
		t.Error("no lock source accepted")
	}
	both := opts(t)
	both.LockOptions = func(supervisor.Spec) proclock.Options { return proclock.Options{} }
	if _, err := New(ctx, probe("yes", "running"), both); err == nil {
		t.Error("LockDir plus LockOptions accepted")
	}
}

func TestLockOptionsReachSetsid(t *testing.T) {
	file := filepath.Join(t.TempDir(), "demo.lock")
	o := opts(t)
	o.LockDir = ""
	o.LockOptions = func(sp supervisor.Spec) proclock.Options { return proclock.Options{File: file, Name: sp.UnitName} }
	sel, err := New(context.Background(), probe("no", "running"), o)
	if err != nil || sel.Mode() != ModeSetsid {
		t.Fatalf("New = %v, %v", sel, err)
	}
	l, err := proclock.Acquire(context.Background(), proclock.Options{File: file, Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Release() }()
	st, err := sel.Status(context.Background(), supervisor.Spec{UnitName: "demo"})
	if err != nil || !st.Running || st.PID != os.Getpid() {
		t.Fatalf("Status = %+v, %v", st, err)
	}
}

func TestWrapDelegates(t *testing.T) {
	f := supervisor.NewFake()
	f.StopErr = errors.New("stop failed")
	sel := Wrap(f)
	if sel.Mode() != ModeBypass || sel.DiagnosticLine() != "" {
		t.Fatalf("Wrap mode=%s line=%q", sel.Mode(), sel.DiagnosticLine())
	}
	spec := supervisor.Spec{UnitName: "demo"}
	if err := sel.Stop(context.Background(), spec); !errors.Is(err, f.StopErr) {
		t.Fatalf("Stop err = %v", err)
	}
	if calls := f.Calls(); len(calls) != 1 || calls[0].Method != "Stop" {
		t.Fatalf("calls = %+v", calls)
	}
}
