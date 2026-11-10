package setsid

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/proclock"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

// helperEnv turns the test binary into a supervised service (see
// TestMain): it acquires the lock, publishes an endpoint and waits.
const helperEnv = "SETSID_TEST_SERVICE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		serve(mode)
		return
	}
	os.Exit(m.Run())
}

func serve(mode string) {
	if mode == "crash" {
		os.Stderr.WriteString("bind: address in use\n") //nolint:errcheck // helper
		os.Exit(3)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	if mode == "stubborn" {
		signal.Ignore(syscall.SIGTERM)
	}
	l, err := proclock.Acquire(context.Background(), helperLock())
	if err != nil {
		os.Exit(1)
	}
	if mode == "handoff" {
		// On SIGTERM: release the lock, linger, re-acquire as a fresh
		// acquisition and keep running. Stop's identity re-check must
		// abort the signal chain instead of escalating to SIGKILL.
		_ = l.Release() //nolint:errcheck // helper
		time.Sleep(20 * time.Millisecond)
		l2, err := proclock.Acquire(context.Background(), helperLock())
		if err != nil {
			os.Exit(2)
		}
		<-sig
		_ = l2.Release() //nolint:errcheck // helper
		return
	}
	<-sig
	_ = l.Release() //nolint:errcheck // helper
}

// helperLock is the lock the helper service takes: an explicit file when
// SETSID_TEST_FILE is set, else the Dir layout.
func helperLock() proclock.Options {
	o := proclock.Options{Name: os.Getenv("SETSID_TEST_NAME"), Endpoint: "unix:///ready"}
	if f := os.Getenv("SETSID_TEST_FILE"); f != "" {
		o.File = f
	} else {
		o.Dir = os.Getenv("SETSID_TEST_DIR")
	}
	return o
}

func spec(t *testing.T, dir, mode string) supervisor.Spec {
	t.Helper()
	t.Setenv(helperEnv, mode)
	t.Setenv("SETSID_TEST_DIR", dir)
	t.Setenv("SETSID_TEST_NAME", "demo:1")
	return supervisor.Spec{
		UnitName: "demo:1", ExePath: os.Args[0], Args: []string{"-test.run=^$"},
		StderrLogPath: filepath.Join(t.TempDir(), "svc.log"),
	}
}

func TestNewRequiresOneLockSource(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("no lock source accepted")
	}
	both := WithLockOptions(func(supervisor.Spec) proclock.Options { return proclock.Options{} })
	if _, err := New(t.TempDir(), both); err == nil {
		t.Fatal("lock dir plus WithLockOptions accepted")
	}
}

func TestLockOptionsLocateCallerPlacedLock(t *testing.T) {
	file := filepath.Join(t.TempDir(), "svc", "locks", "demo.lock")
	s, err := New("", WithStartTimeout(10*time.Second), WithLockOptions(func(sp supervisor.Spec) proclock.Options {
		return proclock.Options{File: file, Name: sp.UnitName}
	}))
	if err != nil {
		t.Fatal(err)
	}
	sp := spec(t, "", "serve")
	t.Setenv("SETSID_TEST_FILE", file)
	ctx := context.Background()
	t.Cleanup(func() { _ = s.Stop(ctx, sp) })
	if err := s.Start(ctx, sp); err != nil {
		t.Fatal(err)
	}
	st, err := s.Status(ctx, sp)
	if err != nil || !st.Running || st.PID <= 0 {
		t.Fatalf("Status = %+v, %v", st, err)
	}
	if err := s.Stop(ctx, sp); err != nil {
		t.Fatal(err)
	}
	if pidAlive(st.PID) {
		t.Fatalf("pid %d alive after Stop", st.PID)
	}
}

func TestStartStatusStop(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, WithStartTimeout(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	sp := spec(t, dir, "serve")
	ctx := context.Background()
	t.Cleanup(func() { _ = s.Stop(ctx, sp) })
	if err := s.Start(ctx, sp); err != nil {
		t.Fatal(err)
	}
	st, err := s.Status(ctx, sp)
	if err != nil || !st.Running || !st.Installed || st.PID <= 0 || st.Mode != Mode {
		t.Fatalf("Status = %+v, %v", st, err)
	}
	if err := s.Start(ctx, sp); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if st2, _ := s.Status(ctx, sp); st2.PID != st.PID {
		t.Fatalf("second Start respawned: %d != %d", st2.PID, st.PID)
	}
	if err := s.Stop(ctx, sp); err != nil {
		t.Fatal(err)
	}
	if pidAlive(st.PID) {
		t.Fatalf("pid %d alive after Stop", st.PID)
	}
	if st, _ := s.Status(ctx, sp); st.Running {
		t.Fatalf("Status after Stop = %+v", st)
	}
	if err := s.Stop(ctx, sp); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}
}

func TestStopEscalatesToSIGKILL(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir, WithKillGrace(200*time.Millisecond))
	sp := spec(t, dir, "stubborn")
	ctx := context.Background()
	t.Cleanup(func() { _ = s.Stop(ctx, sp) })
	if err := s.Start(ctx, sp); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Status(ctx, sp)
	if err := s.Stop(ctx, sp); err != nil {
		t.Fatal(err)
	}
	if pidAlive(st.PID) {
		t.Fatalf("stubborn pid %d survived", st.PID)
	}
}

func TestStopAbortsWhenLockAcquisitionChanges(t *testing.T) {
	// The SIGTERM'd helper releases the lock and re-acquires it as a
	// different acquisition while staying alive: Stop must abort before
	// SIGKILL instead of signaling a lock owner it cannot identify as
	// the one it decided on.
	dir := t.TempDir()
	s, _ := New(dir, WithKillGrace(300*time.Millisecond))
	sp := spec(t, dir, "handoff")
	ctx := context.Background()
	if err := s.Start(ctx, sp); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Status(ctx, sp)
	if err := s.Stop(ctx, sp); err != nil {
		t.Fatalf("Stop err = %v, want nil (chain aborted)", err)
	}
	if !pidAlive(st.PID) {
		t.Fatalf("handoff pid %d was signaled despite the changed acquisition", st.PID)
	}
	if err := syscall.Kill(st.PID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
}

func TestWaitReadyTimeoutAcceptsSlowChild(t *testing.T) {
	// A lock held by a live owner without a published endpoint never
	// satisfies the ready loop, so waitReady runs out its timeout —
	// then treats the lock, held by the pid we spawned, as a success.
	dir := t.TempDir()
	s, _ := New(dir, WithStartTimeout(50*time.Millisecond))
	sp := supervisor.Spec{UnitName: "demo:1"}
	l, err := proclock.Acquire(context.Background(), proclock.Options{Dir: dir, Name: sp.UnitName, PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Release() }()
	err = s.waitReady(context.Background(), sp, os.Getpid(), proclock.Owner{}, false, make(chan error, 1))
	if err != nil {
		t.Fatalf("waitReady err = %v, want success", err)
	}
}

func TestWaitReadyTimeoutWithoutLockFails(t *testing.T) {
	s, _ := New(t.TempDir(), WithStartTimeout(50*time.Millisecond))
	sp := supervisor.Spec{UnitName: "demo:1"}
	err := s.waitReady(context.Background(), sp, os.Getpid(), proclock.Owner{}, false, make(chan error, 1))
	if err == nil || !strings.Contains(err.Error(), "not ready within") {
		t.Fatalf("waitReady err = %v, want timeout", err)
	}
}

func TestWaitReadyTimeoutRejectsStalePreSpawnOwner(t *testing.T) {
	// The lock is still held by the exact acquisition that existed
	// before the spawn (dead owner, reused-PID shape): the timeout
	// recheck must not mistake it for the child we spawned.
	dir := t.TempDir()
	s, _ := New(dir, WithStartTimeout(50*time.Millisecond))
	sp := supervisor.Spec{UnitName: "demo:1"}
	l, err := proclock.Acquire(context.Background(), proclock.Options{Dir: dir, Name: sp.UnitName, PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Release() }()
	err = s.waitReady(context.Background(), sp, os.Getpid(), l.Owner(), true, make(chan error, 1))
	if err == nil || !strings.Contains(err.Error(), "not ready within") {
		t.Fatalf("waitReady err = %v, want timeout", err)
	}
}

func TestStartReportsEarlyExit(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	err := s.Start(context.Background(), spec(t, dir, "crash"))
	if err == nil || !strings.Contains(err.Error(), "exited before it was ready") ||
		!strings.Contains(err.Error(), "address in use") {
		t.Fatalf("Start err = %v", err)
	}
}

func TestInstallUninstallRecordIntent(t *testing.T) {
	s, _ := New(t.TempDir())
	ctx := context.Background()
	sp := supervisor.Spec{UnitName: "demo"}
	_ = s.Install(ctx, sp)
	if st, _ := s.Status(ctx, sp); !st.Installed || st.Running {
		t.Fatalf("after Install: %+v", st)
	}
	_ = s.Uninstall(ctx, sp)
	if st, _ := s.Status(ctx, sp); st.Installed {
		t.Fatalf("after Uninstall: %+v", st)
	}
}
