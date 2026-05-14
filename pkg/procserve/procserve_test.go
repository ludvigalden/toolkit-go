package procserve

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/lockfile"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/sockpath"
)

// TestMain doubles as the server program: a process spawned by
// Client.Connect serves a line-echo handler instead of running tests.
func TestMain(m *testing.M) {
	if Spawned() {
		os.Exit(serveTest())
	}
	os.Exit(m.Run())
}

// serveTest is the spawned server: "pid" answers the PID, "hold" blocks
// until the handler ctx ends, anything else is echoed.
func serveTest() int {
	idle, _ := time.ParseDuration(os.Getenv("PROCSERVE_TEST_IDLE"))
	l, err := Listen(ServerOptions{IdleTimeout: idle, Grace: 300 * time.Millisecond})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	err = Serve(context.Background(), l, HandlerFunc(func(ctx context.Context, conn net.Conn) {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			switch sc.Text() {
			case "pid":
				_, _ = fmt.Fprintln(conn, os.Getpid())
			case "hold":
				<-ctx.Done()
				_, _ = fmt.Fprintln(conn, "canceled")
			default:
				_, _ = fmt.Fprintln(conn, sc.Text())
			}
		}
	}))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// newClient returns a Client spawning this test binary under a short
// private directory, and stops every server it started at cleanup.
func newClient(t *testing.T, idle time.Duration) *Client {
	t.Helper()
	dir, err := os.MkdirTemp("", "ps")
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{
		Dir:         dir,
		FallbackDir: filepath.Join(dir, "fb"),
		Command: func(string) *exec.Cmd {
			cmd := exec.Command(os.Args[0]) //nolint:gosec // re-exec of the test binary
			cmd.Env = append(os.Environ(), "PROCSERVE_TEST_IDLE="+idle.String())
			if f, err := os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				cmd.Stderr = f
			}
			return cmd
		},
	}
	t.Cleanup(func() {
		ctx := context.Background()
		list, _ := c.List(ctx)
		for _, s := range list {
			_ = c.stop(ctx, s.Socket+sockpath.LockSuffix, s.Socket, s.Hash, time.Second, true)
		}
		if log, _ := os.ReadFile(filepath.Join(dir, "server.log")); len(log) > 0 && t.Failed() {
			t.Logf("server log:\n%s", log)
		}
		_ = os.RemoveAll(dir)
	})
	return c
}

// ask sends one line and reads one line back.
func ask(t *testing.T, conn net.Conn, line string) string {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintln(conn, line); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	got, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read reply to %q: %v", line, err)
	}
	return strings.TrimSpace(got)
}

// pidOf connects to identity and asks its server for its PID.
func pidOf(t *testing.T, c *Client, identity string) string {
	t.Helper()
	conn, err := c.Connect(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	return ask(t, conn, "pid")
}

func TestConnectSpawnsAndReuses(t *testing.T) {
	c := newClient(t, time.Minute)
	conn, err := c.Connect(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got := ask(t, conn, "hello"); got != "hello" {
		t.Fatalf("echo = %q", got)
	}
	pid := ask(t, conn, "pid")
	_ = conn.Close()
	if again := pidOf(t, c, "alpha"); again != pid {
		t.Fatalf("second connect reached pid %s, want %s", again, pid)
	}
	if other := pidOf(t, c, "beta"); other == pid {
		t.Fatal("distinct identities share a server")
	}
	st, err := c.Status(context.Background(), "alpha")
	if err != nil || fmt.Sprint(st.PID) != pid || st.Draining {
		t.Fatalf("Status = %+v %v", st, err)
	}
	list, _ := c.List(context.Background())
	if len(list) != 2 {
		t.Fatalf("List = %+v", list)
	}
}

func TestConcurrentConnectsSpawnOnce(t *testing.T) {
	c := newClient(t, time.Minute)
	var wg sync.WaitGroup
	pids := make(chan string, 8)
	for range 8 {
		wg.Go(func() {
			conn, err := c.Connect(context.Background(), "race")
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = conn.Close() }()
			pids <- ask(t, conn, "pid")
		})
	}
	wg.Wait()
	close(pids)
	seen := map[string]bool{}
	for p := range pids {
		seen[p] = true
	}
	if len(seen) != 1 {
		t.Fatalf("servers spawned: %v", seen)
	}
}

func TestIdleTimeoutExitsAndRespawns(t *testing.T) {
	c := newClient(t, 300*time.Millisecond)
	first := pidOf(t, c, "idle")
	p, _ := c.resolvePaths("idle")
	if !lockfile.WaitFree(context.Background(), p.Lock, 5*time.Second) {
		t.Fatal("idle server did not release its lock")
	}
	if _, err := os.Stat(p.Socket); !os.IsNotExist(err) {
		t.Fatalf("socket survived the drain: %v", err)
	}
	if second := pidOf(t, c, "idle"); second == first {
		t.Fatal("expected a fresh server after idle exit")
	}
}

func TestStatusProbesDoNotKeepAlive(t *testing.T) {
	c := newClient(t, 400*time.Millisecond)
	pidOf(t, c, "probe")
	p, _ := c.resolvePaths("probe")
	deadline := time.Now().Add(5 * time.Second)
	for lockfile.Held(p.Lock) && time.Now().Before(deadline) {
		_, _ = c.Status(context.Background(), "probe")
		time.Sleep(50 * time.Millisecond)
	}
	if lockfile.Held(p.Lock) {
		t.Fatal("status probes kept the server alive")
	}
}

func TestShutdownCancelsInFlightAfterGrace(t *testing.T) {
	c := newClient(t, time.Minute)
	conn, err := c.Connect(context.Background(), "drain")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = fmt.Fprintln(conn, "hold")
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	if err := c.Shutdown(context.Background(), "drain", 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(conn).ReadString('\n')
	if strings.TrimSpace(line) != "canceled" {
		t.Fatalf("in-flight handler got %q", line)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("shutdown did not honor the grace")
	}
	if _, err := c.Status(context.Background(), "drain"); err == nil {
		t.Fatal("server answers after Shutdown")
	}
	if err := c.Shutdown(context.Background(), "drain", 0); err != nil {
		t.Fatalf("Shutdown with no server = %v", err)
	}
}

func TestRecoversFromKilledServer(t *testing.T) {
	c := newClient(t, time.Minute)
	first := pidOf(t, c, "crash")
	var pid int
	if _, err := fmt.Sscan(first, &pid); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	p, _ := c.resolvePaths("crash")
	if !lockfile.WaitFree(context.Background(), p.Lock, 5*time.Second) {
		t.Fatal("killed server still holds the lock")
	}
	if _, err := os.Stat(p.Socket); err != nil {
		t.Fatalf("expected a stale socket file: %v", err)
	}
	if second := pidOf(t, c, "crash"); second == first {
		t.Fatal("reconnected to a dead server")
	}
}

func TestSocketPathFallback(t *testing.T) {
	c := newClient(t, time.Minute)
	c.Dir = filepath.Join(c.Dir, strings.Repeat("d", 90))
	conn, err := c.Connect(context.Background(), "long")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	p, _ := c.resolvePaths("long")
	if filepath.Dir(p.Socket) != c.FallbackDir {
		t.Fatalf("socket %s not under fallback %s", p.Socket, c.FallbackDir)
	}
	info, err := os.Stat(c.FallbackDir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("fallback dir mode: %v %v", info, err)
	}
	if got := ask(t, conn, "x"); got != "x" {
		t.Fatalf("echo = %q", got)
	}
}

func TestMaxIdleEvictsLeastRecentlyUsed(t *testing.T) {
	c := newClient(t, time.Minute)
	pidOf(t, c, "old")
	waitIdle(t, c, "old")
	pidOf(t, c, "newer")
	waitIdle(t, c, "newer")
	c.MaxIdle = 2
	pidOf(t, c, "third")
	pOld, _ := c.resolvePaths("old")
	pNewer, _ := c.resolvePaths("newer")
	if lockfile.Held(pOld.Lock) {
		t.Fatal("least recently used server was not evicted")
	}
	if !lockfile.Held(pNewer.Lock) {
		t.Fatal("a more recent server was evicted")
	}
}

// waitIdle waits until the server notices its last connection closed.
func waitIdle(t *testing.T, c *Client, identity string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, err := c.Status(context.Background(), identity); err == nil && st.InFlight == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never went idle", identity)
}

func TestClientValidation(t *testing.T) {
	if _, err := (&Client{Dir: t.TempDir()}).Connect(context.Background(), "x"); err == nil {
		t.Fatal("missing Command accepted")
	}
	if _, err := (&Client{Command: func(string) *exec.Cmd { return nil }}).Connect(context.Background(), "x"); err == nil {
		t.Fatal("missing Dir accepted")
	}
	if _, err := Listen(ServerOptions{}); err == nil {
		t.Fatal("Listen outside a spawned process succeeded")
	}
}

func TestJSONLines(t *testing.T) {
	var buf bytes.Buffer
	in := Message{Op: OpStatus, Identity: "h", Grace: time.Second}
	if err := (JSONLines{}).Write(&buf, in); err != nil {
		t.Fatal(err)
	}
	buf.WriteString("payload")
	br := bufio.NewReader(&buf)
	out, err := (JSONLines{}).Read(br)
	if err != nil || out.Op != in.Op || out.Grace != in.Grace {
		t.Fatalf("round trip = %+v %v", out, err)
	}
	if rest, _ := br.ReadString(0); rest != "payload" {
		t.Fatalf("bytes after the message = %q", rest)
	}
	if _, err := (JSONLines{}).Read(bufio.NewReader(strings.NewReader("{bad\n"))); err == nil {
		t.Fatal("malformed line accepted")
	}
	long := strings.Repeat("x", maxLine+10) + "\n"
	if _, err := (JSONLines{}).Read(bufio.NewReader(strings.NewReader(long))); err == nil {
		t.Fatal("oversized line accepted")
	}
}
