//go:build linux

package systemd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

func testSpec() supervisor.Spec {
	return supervisor.Spec{Namespace: "com.example", UnitName: "demo:8080", ExePath: "/opt/bin/demo", Port: 8080}
}

func newTest(t *testing.T, respond func(string, []string) ([]byte, []byte, error), opts ...Option) (*Systemd, *supervisor.FakeExecutor, string) {
	t.Helper()
	home := t.TempDir()
	fx := supervisor.NewFakeExecutor(respond)
	getenv := func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	s, err := New(fx, append([]Option{WithGetenv(getenv)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	s.pollInterval = time.Millisecond
	return s, fx, filepath.Join(home, ".config", "systemd", "user")
}

func joined(fx *supervisor.FakeExecutor) []string {
	var out []string
	for _, c := range fx.Calls() {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func TestUnitFileName(t *testing.T) {
	got, err := UnitFileName(testSpec())
	if err != nil || got != "com.example.demo.8080.service" {
		t.Fatalf("UnitFileName = %q, %v", got, err)
	}
}

func TestNewRejectsNilExecutor(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("want error")
	}
}

func TestInstallIdempotent(t *testing.T) {
	s, fx, dir := newTest(t, nil)
	ctx := context.Background()
	for range 2 {
		if err := s.Install(ctx, testSpec()); err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "com.example.demo.8080.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Environment=PATH=/opt/bin:"+SystemPATH+"\n") {
		t.Errorf("unit PATH not canonical:\n%s", body)
	}
	if got := joined(fx); len(got) != 1 || got[0] != "systemctl --user daemon-reload" {
		t.Errorf("calls = %q, want one daemon-reload", got)
	}
}

func TestStartPollsUntilActive(t *testing.T) {
	polls := 0
	markers := supervisor.Markers{Dir: t.TempDir()}
	s, _, _ := newTest(t, func(_ string, args []string) ([]byte, []byte, error) {
		if len(args) > 1 && args[1] == "show" && strings.Contains(strings.Join(args, " "), "Result") {
			if polls++; polls < 3 {
				return []byte("ActiveState=activating\nSubState=start\n"), nil, nil
			}
			return []byte("ActiveState=active\nSubState=running\n"), nil, nil
		}
		return []byte("active running"), nil, nil
	}, WithMarkers(markers))
	if err := markers.Write("com.example.demo.8080.service", "demo", supervisor.MaintenanceReasonRestart, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), testSpec()); err != nil {
		t.Fatal(err)
	}
	if polls != 3 {
		t.Errorf("polls = %d, want 3", polls)
	}
	if _, found, _ := markers.Read("com.example.demo.8080.service"); found {
		t.Error("Start left the maintenance marker")
	}
}

func TestStartFailsFast(t *testing.T) {
	s, _, _ := newTest(t, func(_ string, args []string) ([]byte, []byte, error) {
		if strings.Contains(strings.Join(args, " "), "--value") {
			return []byte("failed\nfailed\n"), nil, nil
		}
		return []byte("ActiveState=failed\nResult=exit-code\nExecMainStatus=2\n"), nil, nil
	})
	err := s.Start(context.Background(), testSpec())
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("Start err = %v", err)
	}
}

func TestStopWritesMarker(t *testing.T) {
	markers := supervisor.Markers{Dir: t.TempDir()}
	s, fx, _ := newTest(t, nil, WithMarkers(markers))
	ctx := supervisor.WithMaintenanceReason(context.Background(), supervisor.MaintenanceReasonRecover)
	if err := s.Stop(ctx, testSpec()); err != nil {
		t.Fatal(err)
	}
	m, found, err := markers.Read("com.example.demo.8080.service")
	if err != nil || !found || m.Reason != "recover" || m.Tool != "demo" {
		t.Fatalf("marker = %+v %v %v", m, found, err)
	}
	if got := joined(fx); len(got) != 1 || got[0] != "systemctl --user stop com.example.demo.8080" {
		t.Errorf("calls = %q", got)
	}
}

func TestStatus(t *testing.T) {
	s, _, _ := newTest(t, func(string, []string) ([]byte, []byte, error) {
		return []byte("ActiveState=active\nSubState=running\nExecMainPID=4242\nExecMainStatus=0\nResult=success\n"), nil, nil
	})
	st, err := s.Status(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.PID != 4242 || st.Installed || st.Mode != Mode || st.Diagnostic != "" {
		t.Fatalf("Status = %+v", st)
	}

	s, _, _ = newTest(t, func(string, []string) ([]byte, []byte, error) {
		return []byte("ActiveState=failed\nSubState=failed\nExecMainPID=0\nResult=start-limit-hit\n"), nil, nil
	})
	st, _ = s.Status(context.Background(), testSpec())
	if st.Running || !strings.Contains(st.Diagnostic, "Result=start-limit-hit") {
		t.Fatalf("failed Status = %+v", st)
	}

	s, _, _ = newTest(t, func(string, []string) ([]byte, []byte, error) { return nil, nil, errors.New("boom") })
	if st, err := s.Status(context.Background(), testSpec()); err != nil || st.Running {
		t.Fatalf("unloaded Status = %+v, %v", st, err)
	}
}

func TestUninstallToleratesUnloaded(t *testing.T) {
	s, fx, dir := newTest(t, func(_ string, args []string) ([]byte, []byte, error) {
		if args[1] == "disable" {
			return nil, []byte("Unit com.example.demo.8080.service not loaded."), errors.New("exit status 1")
		}
		return nil, nil, nil
	})
	if err := s.Install(context.Background(), testSpec()); err != nil {
		t.Fatal(err)
	}
	if err := s.Uninstall(context.Background(), testSpec()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "com.example.demo.8080.service")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unit file still present: %v", err)
	}
	if got := joined(fx); got[len(got)-1] != "systemctl --user daemon-reload" {
		t.Errorf("calls = %q", got)
	}
}

func TestAvailable(t *testing.T) {
	for _, tc := range []struct {
		linger, state string
		want          bool
	}{
		{"Linger=yes", "running", true},
		{"Linger=yes", "degraded", true},
		{"Linger=no", "running", false},
		{"Linger=yes", "offline", false},
	} {
		s, _, _ := newTest(t, func(name string, _ []string) ([]byte, []byte, error) {
			if name == "loginctl" {
				return []byte(tc.linger + "\n"), nil, nil
			}
			return []byte(tc.state + "\n"), nil, errors.New("exit status 1")
		})
		if got := s.Available(context.Background()); got != tc.want {
			t.Errorf("Available(%s, %s) = %v", tc.linger, tc.state, got)
		}
	}
}
