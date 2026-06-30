package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestIdentity(t *testing.T) {
	for _, tc := range []struct {
		spec    Spec
		want    string
		wantErr bool
	}{
		{Spec{Namespace: "com.example", UnitName: "demo", Port: 0}, "com.example.demo.0", false},
		{Spec{Namespace: "com.example", UnitName: "demo:3", Port: 8080}, "com.example.demo.8080", false},
		{Spec{Namespace: "", UnitName: "demo"}, "", true},
		{Spec{Namespace: "com.example", UnitName: ":3"}, "", true},
		{Spec{Namespace: "com.example", UnitName: "a/b"}, "", true},
		{Spec{Namespace: "com example", UnitName: "demo"}, "", true},
	} {
		got, err := tc.spec.Identity()
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%+v: Identity = %q, %v", tc.spec, got, err)
		}
	}
	if Identity("com.example", "demo", 443) != "com.example.demo.443" {
		t.Error("Identity func mismatch")
	}
}

func TestSpecJSONRoundTrip(t *testing.T) {
	oom := -1000
	in := Spec{
		Namespace: "com.example", UnitName: "demo:1", ExePath: "/bin/demo", Args: []string{"serve"},
		Port: 1, StderrLogPath: "/l", WorkingDirectory: "/w", OOMScoreAdjust: &oom,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"namespace":"com.example","unit_name":"demo:1","exe_path":"/bin/demo","args":["serve"],"port":1,` +
		`"stderr_log_path":"/l","working_directory":"/w","oom_score_adjust":-1000}`
	if string(b) != want {
		t.Fatalf("wire format changed:\n%s", b)
	}
	var out Spec
	if err := json.Unmarshal(b, &out); err != nil || *out.OOMScoreAdjust != oom || out.UnitName != in.UnitName {
		t.Fatalf("round trip = %+v, %v", out, err)
	}
}

func TestFakeRecordsAndReturns(t *testing.T) {
	f := NewFake()
	f.StatusReturn = Status{Running: true, Mode: "fake"}
	f.StartErr = errors.New("start")
	ctx := context.Background()
	spec := Spec{UnitName: "demo", Args: []string{"a"}}
	_ = f.Install(ctx, spec)
	if err := f.Start(ctx, spec); !errors.Is(err, f.StartErr) {
		t.Errorf("Start err = %v", err)
	}
	spec.Args[0] = "mutated"
	_ = f.Stop(ctx, spec)
	st, _ := f.Status(ctx, spec)
	_ = f.Uninstall(ctx, spec)
	if !st.Running {
		t.Error("StatusReturn ignored")
	}
	calls := f.Calls()
	var methods []string
	for _, c := range calls {
		methods = append(methods, c.Method)
	}
	if !slices.Equal(methods, []string{"Install", "Start", "Stop", "Status", "Uninstall"}) {
		t.Fatalf("methods = %v", methods)
	}
	if calls[0].Spec.Args[0] != "a" {
		t.Error("recorded Args aliased the caller's slice")
	}
	f.Err = errors.New("all")
	if err := f.Install(ctx, spec); !errors.Is(err, f.Err) {
		t.Errorf("Install err = %v", err)
	}
}

func TestFakeExecutorRoundTrip(t *testing.T) {
	fx := NewFakeExecutor(func(name string, args []string) ([]byte, []byte, error) {
		if name == "fail" {
			return nil, []byte("bad"), errors.New("exit 1")
		}
		return []byte(strings.Join(args, ",")), nil, nil
	})
	ctx := context.Background()
	out, _, err := fx.Run(ctx, "echo", "a", "b")
	if err != nil || string(out) != "a,b" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	if _, se, err := fx.Run(ctx, "fail"); err == nil || string(se) != "bad" {
		t.Fatalf("fail Run = %q, %v", se, err)
	}
	if got := fx.Calls(); len(got) != 2 || !slices.Equal(got[0], []string{"echo", "a", "b"}) {
		t.Fatalf("Calls = %v", got)
	}
	if _, _, err := NewFakeExecutor(nil).Run(ctx, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestOSExecutor(t *testing.T) {
	out, _, err := NewOSExecutor().Run(context.Background(), "/bin/sh", "-c", "printf hi")
	if err != nil || string(out) != "hi" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	_, se, err := NewOSExecutor().Run(context.Background(), "/bin/sh", "-c", "printf oops >&2; exit 3")
	if err == nil || string(se) != "oops" {
		t.Fatalf("failing Run = %q, %v", se, err)
	}
}

func TestJournalTail(t *testing.T) {
	fx := NewFakeExecutor(func(string, []string) ([]byte, []byte, error) { return []byte("line\n"), nil, nil })
	if got := JournalTail(context.Background(), fx, "u.service", 5); got != "line\n" {
		t.Fatalf("JournalTail = %q", got)
	}
	want := []string{"journalctl", "--user", "-u", "u.service", "--no-pager", "-n", "5"}
	if c := fx.Calls(); len(c) != 1 || !slices.Equal(c[0], want) {
		t.Fatalf("Calls = %v", c)
	}
	if JournalTail(context.Background(), nil, "u", 5) != "" {
		t.Error("nil executor produced output")
	}
}

func TestMarkersWriteReadClear(t *testing.T) {
	m := Markers{Dir: filepath.Join(t.TempDir(), "maintenance")}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := m.Write("com.example.demo.0.service", "demo", MaintenanceReasonRestart, now); err != nil {
		t.Fatal(err)
	}
	mk, found, err := m.Read("com.example.demo.0.service")
	if err != nil || !found {
		t.Fatalf("Read = %v %v", found, err)
	}
	if mk.Unit != "com.example.demo.0.service" || mk.Tool != "demo" || mk.Reason != "restart" ||
		!mk.Expires.Equal(now.Add(MaintenanceMarkerTTL)) || !mk.Active(now) || mk.Active(mk.Expires) {
		t.Fatalf("marker = %+v", mk)
	}
	if err := m.Clear("com.example.demo.0.service"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := m.Read("com.example.demo.0.service"); found {
		t.Fatal("marker survived Clear")
	}
	if err := m.Clear("com.example.demo.0.service"); err != nil {
		t.Fatalf("Clear of absent marker: %v", err)
	}
}

func TestMarkersSweepAndDisabled(t *testing.T) {
	m := Markers{Dir: t.TempDir()}
	old := time.Now().Add(-time.Hour)
	if err := m.Write("old", "t", MaintenanceReasonStop, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "junk.marker"), []byte("not a marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Write("new", "t", MaintenanceReasonStop, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := m.Read("old"); found {
		t.Error("expired marker not swept")
	}
	if _, err := os.Stat(filepath.Join(m.Dir, "junk.marker")); err != nil {
		t.Error("malformed marker was swept")
	}
	if err := m.Write("../escape", "t", MaintenanceReasonStop, time.Now()); err == nil {
		t.Error("path-like unit accepted")
	}

	var off Markers
	if err := off.Write("u", "t", MaintenanceReasonStop, time.Now()); err != nil {
		t.Error(err)
	}
	if _, found, err := off.Read("u"); found || err != nil {
		t.Error("disabled markers read something")
	}
}

func TestRuntimeMarkers(t *testing.T) {
	getenv := func(k string) string { return map[string]string{"XDG_RUNTIME_DIR": "/run/user/1"}[k] }
	m, err := RuntimeMarkers("demo", getenv)
	if err != nil || m.Dir != "/run/user/1/demo/maintenance" {
		t.Fatalf("Dir = %q, %v", m.Dir, err)
	}
	if m, err := RuntimeMarkers("demo", func(string) string { return "" }); err != nil || m.Dir != "" {
		t.Fatalf("no runtime dir: Dir = %q, %v", m.Dir, err)
	}
	if _, err := RuntimeMarkers("bad/name", getenv); err == nil {
		t.Fatal("RuntimeMarkers accepted an invalid app name")
	}
}

func TestMaintenanceReasonContext(t *testing.T) {
	ctx := context.Background()
	if MaintenanceReasonFromContext(ctx) != "" {
		t.Fatal("undecorated ctx has a reason")
	}
	if MaintenanceReasonFromContext(WithMaintenanceReason(ctx, MaintenanceReasonRecover)) != MaintenanceReasonRecover {
		t.Fatal("reason lost")
	}
}
