//go:build darwin

package launchd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

func testSpec() supervisor.Spec {
	return supervisor.Spec{Namespace: "com.example", UnitName: "demo:8080", ExePath: "/opt/bin/demo", Port: 8080}
}

// newTest wires a Launchd over a FakeExecutor answering id -u with 501 so
// the gui target is deterministic, unless respond overrides it.
func newTest(t *testing.T, respond func(string, []string) ([]byte, []byte, error), opts ...Option) (*Launchd, *supervisor.FakeExecutor, string) {
	t.Helper()
	if respond == nil {
		respond = func(name string, _ []string) ([]byte, []byte, error) {
			if name == "id" {
				return []byte("501\n"), nil, nil
			}
			return nil, nil, nil
		}
	}
	fx := supervisor.NewFakeExecutor(respond)
	agents := t.TempDir()
	opts = append([]Option{WithAgentsDir(agents), WithLogDir(t.TempDir())}, opts...)
	l, err := New(fx, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return l, fx, agents
}

func joined(fx *supervisor.FakeExecutor) []string {
	var out []string
	for _, c := range fx.Calls() {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func plistBody(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallFreshWritesPlistWithoutLaunchctl(t *testing.T) {
	l, fx, dir := newTest(t, nil)
	if err := l.Install(context.Background(), testSpec()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "com.example.demo.8080.plist")); err != nil {
		t.Fatal(err)
	}
	if got := joined(fx); len(got) != 0 {
		t.Errorf("calls = %q, want none on a fresh install", got)
	}
}

func TestInstallIdempotent(t *testing.T) {
	l, fx, _ := newTest(t, nil)
	ctx := context.Background()
	for range 2 {
		if err := l.Install(ctx, testSpec()); err != nil {
			t.Fatal(err)
		}
	}
	if got := joined(fx); len(got) != 0 {
		t.Errorf("calls = %q, want none for a byte-identical reinstall", got)
	}
}

func TestInstallReloadsChangedPlistOnBootstrappedAgent(t *testing.T) {
	// launchctl print succeeds, so the agent is bootstrapped: the changed
	// plist must propagate through bootout + bootstrap, the only way
	// launchd re-reads a plist.
	l, fx, dir := newTest(t, func(name string, args []string) ([]byte, []byte, error) {
		if name == "id" {
			return []byte("501\n"), nil, nil
		}
		if name == "launchctl" && args[0] == "print" {
			return []byte("state = running\n"), nil, nil
		}
		return nil, nil, nil
	})
	ctx := context.Background()
	if err := l.Install(ctx, testSpec()); err != nil {
		t.Fatal(err)
	}
	changed := testSpec()
	changed.Args = []string{"--verbose"}
	if err := l.Install(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plistBody(t, dir, "com.example.demo.8080.plist"), "--verbose") {
		t.Error("changed plist was not written")
	}
	want := []string{
		"launchctl print gui/501/com.example.demo.8080",
		"launchctl bootout gui/501/com.example.demo.8080",
		"launchctl bootstrap gui/501 " + filepath.Join(dir, "com.example.demo.8080.plist"),
	}
	if got := joined(fx); len(got) != len(want) {
		t.Fatalf("calls = %q, want %q", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("call %d = %q, want %q", i, got[i], want[i])
			}
		}
	}
}

func TestInstallLeavesDormantAgentUnreloaded(t *testing.T) {
	// launchctl print fails, so the agent is not bootstrapped: the new
	// plist is written but no bootout/bootstrap runs.
	l, fx, _ := newTest(t, func(name string, args []string) ([]byte, []byte, error) {
		if name == "id" {
			return []byte("501\n"), nil, nil
		}
		if name == "launchctl" && args[0] == "print" {
			return nil, []byte("Could not find service \"gui/501/com.example.demo.8080\" in domain"), errors.New("exit status 3")
		}
		return nil, nil, nil
	})
	ctx := context.Background()
	if err := l.Install(ctx, testSpec()); err != nil {
		t.Fatal(err)
	}
	changed := testSpec()
	changed.Args = []string{"--verbose"}
	if err := l.Install(ctx, changed); err != nil {
		t.Fatal(err)
	}
	got := joined(fx)
	if len(got) != 1 || !strings.HasPrefix(got[0], "launchctl print ") {
		t.Fatalf("calls = %q, want only the bootstrapped probe", got)
	}
}

func TestStopWritesAndStartClearsMarker(t *testing.T) {
	markers := supervisor.Markers{Dir: t.TempDir()}
	l, _, _ := newTest(t, func(name string, args []string) ([]byte, []byte, error) {
		if name == "id" {
			return []byte("501\n"), nil, nil
		}
		if name == "launchctl" && args[0] == "print" {
			return []byte("state = running\n"), nil, nil
		}
		return nil, nil, nil
	}, WithMarkers(markers))
	ctx := supervisor.WithMaintenanceReason(context.Background(), supervisor.MaintenanceReasonRestart)
	if err := l.Stop(ctx, testSpec()); err != nil {
		t.Fatal(err)
	}
	m, found, err := markers.Read("com.example.demo.8080")
	if err != nil || !found || m.Reason != "restart" || m.Tool != "demo" || m.Unit != "com.example.demo.8080" {
		t.Fatalf("marker = %+v, found %v, err %v", m, found, err)
	}
	if err := l.Start(context.Background(), testSpec()); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := markers.Read("com.example.demo.8080"); found {
		t.Error("Start left the maintenance marker")
	}
}
