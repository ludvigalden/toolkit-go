//go:build linux

package selector

import (
	"context"
	"errors"
	"strings"
	"testing"

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
		t.Error("empty LockDir accepted")
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
