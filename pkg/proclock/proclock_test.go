package proclock

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helperEnv makes the test binary act as a lock holder (see TestMain).
const helperEnv = "PROCLOCK_TEST_HOLDER"

func TestMain(m *testing.M) {
	if dir := os.Getenv(helperEnv); dir != "" {
		holdLock(dir)
		return
	}
	os.Exit(m.Run())
}

// holdLock acquires "shared" under dir, reports readiness on stdout, and
// holds the lock until killed.
func holdLock(dir string) {
	l, err := Acquire(context.Background(), Options{Dir: dir, Name: "shared", Endpoint: "unix:///holder"})
	if err != nil {
		os.Stdout.WriteString("ERR " + err.Error() + "\n") //nolint:errcheck // helper
		os.Exit(1)
	}
	os.Stdout.WriteString("READY\n") //nolint:errcheck // helper
	_ = l
	select {}
}

// startHolder runs the helper process and waits until it holds the lock.
func startHolder(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0]) //nolint:gosec // re-exec of the test binary
	cmd.Env = append(os.Environ(), helperEnv+"="+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, _ := bufio.NewReader(out).ReadString('\n')
	if strings.TrimSpace(line) != "READY" {
		t.Fatalf("holder: %q", line)
	}
	return cmd
}

func TestAcquireReleaseAndGenerations(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Dir: dir, Name: "svc"}
	l, err := Acquire(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if o := l.Owner(); o.PID != os.Getpid() || o.Generation != 1 || !o.Starting() || o.Nonce == "" {
		t.Fatalf("owner = %+v", o)
	}
	if err := l.Publish("tcp://127.0.0.1:9", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	got, found, err := Read(opts)
	if err != nil || !found || got.Endpoint != "tcp://127.0.0.1:9" || got.Extra["k"] != "v" {
		t.Fatalf("Read = %+v %v %v", got, found, err)
	}
	// A second open file description contends even in-process.
	_, err = Acquire(context.Background(), opts)
	var held *HeldError
	if !errors.As(err, &held) || !errors.Is(err, ErrHeld) || held.Owner.PID != os.Getpid() {
		t.Fatalf("second Acquire = %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Fatal("Release not idempotent")
	}
	if err := l.Publish("x", nil); !errors.Is(err, ErrNotHeld) {
		t.Fatalf("Publish after Release = %v", err)
	}
	if _, found, _ := Read(opts); found {
		t.Fatal("lock file survived Release")
	}
}

func TestOptionsValidationAndRepoScope(t *testing.T) {
	if _, err := Acquire(context.Background(), Options{Name: "x"}); err == nil {
		t.Fatal("missing Dir accepted")
	}
	dir := t.TempDir()
	repo := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	a, _ := Options{Dir: dir, Name: "n", Repo: repo}.Path()
	b, _ := Options{Dir: dir, Name: "n", Repo: link}.Path()
	g, _ := Options{Dir: dir, Name: "n"}.Path()
	if a != b || a == g || !strings.HasPrefix(a, dir) {
		t.Fatalf("paths: repo=%s alias=%s global=%s", a, b, g)
	}
}

func TestWaitModeAndWaitUnlock(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Dir: dir, Name: "w"}
	l, err := Acquire(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	waitOpts := opts
	waitOpts.Wait = true
	if _, err := Acquire(ctx, waitOpts); !errors.Is(err, ErrTimeout) {
		t.Fatalf("bounded wait = %v", err)
	}
	time.AfterFunc(100*time.Millisecond, func() { _ = l.Release() })
	l2, err := Acquire(context.Background(), Options{Dir: dir, Name: "w", Wait: true, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("wait-mode Acquire = %v", err)
	}
	if l2.Owner().Generation != 1 {
		t.Fatalf("generation after clean release = %d", l2.Owner().Generation)
	}
	time.AfterFunc(100*time.Millisecond, func() { _ = l2.Release() })
	if err := WaitUnlock(context.Background(), Options{Dir: dir, Name: "w", Timeout: 5 * time.Second}); err != nil {
		t.Fatal(err)
	}
}

func TestWaitUnlockTimeoutPrecedence(t *testing.T) {
	dir := t.TempDir()
	holder := startHolder(t, dir)
	defer func() {
		_ = holder.Process.Kill()
		_ = holder.Wait()
	}()
	// An existing ctx deadline wins over Timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := WaitUnlock(ctx, Options{Dir: dir, Name: "shared", Timeout: 30 * time.Second})
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 5*time.Second {
		t.Fatalf("deadline vs Timeout: err = %v after %v", err, time.Since(start))
	}
	// With no deadline, Timeout governs.
	start = time.Now()
	err = WaitUnlock(context.Background(), Options{Dir: dir, Name: "shared", Timeout: 200 * time.Millisecond})
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 5*time.Second {
		t.Fatalf("Timeout only: err = %v after %v", err, time.Since(start))
	}
}

func TestCrossProcessHoldAndReclaim(t *testing.T) {
	dir := t.TempDir()
	holder := startHolder(t, dir)
	opts := Options{Dir: dir, Name: "shared"}
	_, err := Acquire(context.Background(), opts)
	var held *HeldError
	if !errors.As(err, &held) || held.Owner.PID != holder.Process.Pid || held.Owner.Endpoint != "unix:///holder" {
		t.Fatalf("Acquire vs live holder = %v", err)
	}
	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = holder.Wait()
	opts.Wait, opts.Timeout = true, 5*time.Second
	l, err := Acquire(context.Background(), opts)
	if err != nil {
		t.Fatalf("reclaim after SIGKILL = %v", err)
	}
	defer l.Release() //nolint:errcheck // test cleanup
	if g := l.Owner().Generation; g != 2 {
		t.Fatalf("generation after takeover = %d, want 2", g)
	}
}

func TestCorruptAndFutureSchema(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Dir: dir, Name: "c"}
	path, _ := opts.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// An unheld corrupt file is simply taken over.
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(context.Background(), opts)
	if err != nil {
		t.Fatalf("Acquire over unheld corrupt file = %v", err)
	}
	_ = l.Release()

	if err := os.WriteFile(path, []byte(`{"schema":99,"name":"c","pid":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o, found, err := Read(opts)
	if err != nil || !found || o.Schema != 99 {
		t.Fatalf("Read future schema = %+v %v %v", o, found, err)
	}
}

func TestSweepRemovesStale(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "global", "dead.lock")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	body, _ := Owner{Schema: 1, Name: "dead", PID: 1 << 30}.encode()
	if err := os.WriteFile(stale, body, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(context.Background(), Options{Dir: dir, Name: "live"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release() //nolint:errcheck // test cleanup
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale lock survived the sweep: %v", err)
	}
}
