package procrun

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// writeScript writes an executable shell script and returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // test executable
		t.Fatal(err)
	}
	return p
}

func TestRunCapturesOutput(t *testing.T) {
	res, err := Cmd("sh", "-c", "echo out; echo err >&2").Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(res.Stdout) != "out\n" || string(res.Stderr) != "err\n" || res.ExitCode != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestRunExitError(t *testing.T) {
	res, err := Cmd("sh", "-c", "echo 'zle: noise' >&2; echo boom >&2; exit 3").Run(context.Background())
	var ee *ExitError
	if !errors.As(err, &ee) || ee.ExitCode != 3 || res.ExitCode != 3 {
		t.Fatalf("err = %v, res = %+v", err, res)
	}
	if msg := ee.Error(); !strings.HasSuffix(msg, ": boom") || strings.Contains(msg, "zle") {
		t.Fatalf("message = %q", msg)
	}
	if !strings.Contains(string(ee.Stderr), "zle") {
		t.Fatal("raw stderr lost the noise line")
	}
}

func TestRunStdinDirEnv(t *testing.T) {
	dir := t.TempDir()
	rc := NewRunContext(dir, WithEnv("PROCRUN_T=v1"))
	res, err := rc.Cmd("sh", "-c", `cat; pwd; echo "$PROCRUN_T"`).Stdin(strings.NewReader("in\n")).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	lines := strings.Split(strings.TrimSpace(string(res.Stdout)), "\n")
	got, _ := filepath.EvalSymlinks(lines[1])
	if lines[0] != "in" || got != want || lines[2] != "v1" {
		t.Fatalf("stdout = %q", res.Stdout)
	}
}

func TestTimeoutKillsGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "grandchild.pid")
	start := time.Now()
	_, err := Cmd("sh", "-c", "sleep 30 & echo $! > "+marker+"; wait").Timeout(200 * time.Millisecond).Run(context.Background())
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TimeoutError", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout did not bound the run")
	}
	raw, rerr := os.ReadFile(marker)
	if rerr != nil {
		t.Fatal(rerr)
	}
	pid := atoi(strings.TrimSpace(string(raw)))
	waitGone(t, pid)
}

func TestGracefulStopDeliversTerm(t *testing.T) {
	_, err := Cmd("sh", "-c", `trap 'echo term; exit 0' TERM; while :; do sleep 0.05; done`).
		Timeout(200 * time.Millisecond).GracefulStop(3 * time.Second).Run(context.Background())
	var te *TimeoutError
	if !errors.As(err, &te) || te.Grace != 3*time.Second {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(te.Error(), "graceful stop") {
		t.Fatalf("message = %q", te.Error())
	}
}

func TestStreamJSONAndEarlyStop(t *testing.T) {
	proc, err := Cmd("sh", "-c", `echo '{"a":1}'; echo plain; echo '[1]'`).Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var kinds []EventKind
	var exit Event
	werr := StreamJSON(proc, func(e Event) bool {
		kinds = append(kinds, e.Kind)
		if e.Kind == KindJSON && e.Line == 1 && e.Data["a"] != float64(1) {
			t.Errorf("data = %v", e.Data)
		}
		if e.Kind == KindExit {
			exit = e
		}
		return true
	})
	if werr != nil || exit.ExitCode != 0 {
		t.Fatalf("err = %v exit = %+v", werr, exit)
	}
	want := []EventKind{KindJSON, KindNonJSON, KindJSON, KindExit}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v", kinds)
		}
	}

	proc, err = Cmd("sh", "-c", "echo one; exec sleep 30").Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	_ = StreamLines(proc, func(e Event) bool {
		seen++
		return e.Kind != KindLine
	})
	if seen != 2 {
		t.Fatalf("events = %d, want line + exit", seen)
	}
}

func TestParseLines(t *testing.T) {
	evs := ParseLines([]byte("{\"k\":\"v\"}\n\n  \nnope\n"))
	if len(evs) != 2 || evs[0].Kind != KindJSON || evs[1].Kind != KindNonJSON || evs[1].Line != 2 {
		t.Fatalf("events = %+v", evs)
	}
}

func TestRawReader(t *testing.T) {
	proc, err := Cmd("printf", "abc").Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	for chunk, ok := proc.Raw().Next(); ok; chunk, ok = proc.Raw().Next() {
		got = append(got, chunk...)
	}
	if _, err := proc.Wait(); err != nil {
		t.Fatal(err)
	}
	if string(got) != "abc" {
		t.Fatalf("raw = %q", got)
	}
	if _, err := proc.Wait(); !errors.As(err, new(*DoubleWaitError)) {
		t.Fatalf("second Wait = %v", err)
	}
}

func TestFindStages(t *testing.T) {
	dir := t.TempDir()
	tool := writeScript(t, dir, "mytool", "exit 0")
	rc := NewRunContext("", WithResolver(resolverFunc(func(name string) (string, bool) {
		if name == "alias" {
			return tool, true
		}
		if name == "relative" {
			return "rel/path", true
		}
		return "", false
	})))
	if p, err := rc.Find(tool); err != nil || p != tool {
		t.Fatalf("absolute: %q %v", p, err)
	}
	if p, err := rc.Find("alias"); err != nil || p != tool {
		t.Fatalf("resolver: %q %v", p, err)
	}
	if p, err := rc.Find("sh"); err != nil || !filepath.IsAbs(p) {
		t.Fatalf("PATH: %q %v", p, err)
	}
	t.Setenv("SHELL", "")
	_, err := rc.FindWithoutShell("relative")
	var nf *ToolNotFoundError
	if !errors.As(err, &nf) || nf.Name != "relative" {
		t.Fatalf("relative resolver result accepted: %v", err)
	}
	if _, err := Find("definitely-not-a-tool-xyz"); !errors.As(err, &nf) {
		t.Fatalf("missing tool: %v", err)
	}
}

func TestShellProbeAndFallback(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "hidden")
	if err := os.Mkdir(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := writeScript(t, toolDir, "hiddentool", `echo "ran $*"`)
	// The fake login shell sees an extra PATH entry and reports the marker.
	shell := writeScript(t, dir, "fakeshell", `[ "$MY_CHILD" = 1 ] || exit 9
PATH="`+toolDir+`:$PATH" exec /bin/sh "$@"`)
	t.Setenv("SHELL", shell)

	var mu sync.Mutex
	var events []DiagKind
	rc := NewRunContext("", WithShellChildEnv("MY_CHILD"), WithObserver(func(e DiagEvent) {
		mu.Lock()
		events = append(events, e.Kind)
		mu.Unlock()
	}))
	p, err := rc.Find("hiddentool")
	if err != nil || p != tool {
		t.Fatalf("probe = %q %v", p, err)
	}
	if _, err := rc.Cmd("hiddentool", "x").Run(context.Background()); err == nil {
		t.Fatal("run without ShellFallback should fail resolution")
	}
	res, err := rc.Cmd("hiddentool", "a b").ShellFallback().Run(context.Background())
	if err != nil || string(res.Stdout) != "ran a b\n" {
		t.Fatalf("fallback = %q %v", res.Stdout, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if count(events, DiagShellSpawn) != 2 || count(events, DiagExit) != 1 || count(events, DiagLaunch) != 1 {
		t.Fatalf("events = %v", events)
	}
}

func TestRegistryCleanup(t *testing.T) {
	reg := NewRegistry()
	rc := NewRunContext("", WithRegistry(reg))
	long, err := rc.Cmd("sh", "-c", `trap 'exit 0' TERM; while :; do sleep 0.05; done`).LongRunning().Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	short, err := rc.Cmd("sleep", "30").Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reg.Len() != 2 {
		t.Fatalf("Len = %d", reg.Len())
	}
	done := make(chan struct{})
	go func() {
		_, _ = long.Wait()
		_, _ = short.Wait()
		close(done)
	}()
	reg.Cleanup(2 * time.Second)
	reg.Cleanup(0)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup left processes running")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len after cleanup = %d", reg.Len())
	}
	stop := reg.CleanupOnSignal(time.Second)
	if again := reg.CleanupOnSignal(time.Second); again == nil {
		t.Fatal("nil stop")
	}
	stop()
	stop()
}

func TestScoperAndDerive(t *testing.T) {
	var gotProfile string
	released := false
	parent := NewRunContext("")
	child := parent.Derive(WithScoper(ScoperFunc(func(_ context.Context, profile string, cmd *exec.Cmd) (func(context.Context) error, error) {
		gotProfile = profile
		cmd.Env = append(os.Environ(), "SCOPED=1")
		return func(context.Context) error { released = true; return nil }, nil
	})))
	if parent.scoper != nil {
		t.Fatal("Derive mutated the parent")
	}
	res, err := child.Cmd("sh", "-c", `echo "$SCOPED"`).Profile("low").Run(context.Background())
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "1" || gotProfile != "low" || !released {
		t.Fatalf("scoped run: %q %v profile=%q released=%v", res.Stdout, err, gotProfile, released)
	}
	pc, err := child.Cmd("true").Profile("p").Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := pc.Cmd().Run(); err != nil {
		t.Fatal(err)
	}
	if err := pc.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	failing := parent.Derive(WithScoper(ScoperFunc(func(context.Context, string, *exec.Cmd) (func(context.Context) error, error) {
		return nil, errors.New("denied")
	})))
	if _, err := failing.Cmd("true").Profile("p").Run(context.Background()); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("scoper error = %v", err)
	}
}

func TestSiblingResolver(t *testing.T) {
	dir := t.TempDir()
	want := writeScript(t, dir, "tool-"+runtime.GOARCH, "exit 0")
	if p, ok := (SiblingResolver{BaseDir: dir}).Resolve("tool"); !ok || p != want {
		t.Fatalf("Resolve = %q %v", p, ok)
	}
	if _, ok := (SiblingResolver{BaseDir: dir}).Resolve("absent"); ok {
		t.Fatal("absent tool resolved")
	}
}

func TestReadStats(t *testing.T) {
	s, err := ReadStats(os.Getpid())
	if err != nil || s.RSSBytes == 0 || s.PID != os.Getpid() || s.Name == "" {
		t.Fatalf("self stats = %+v %v", s, err)
	}
	if _, err := ReadStats(1 << 30); err == nil {
		t.Fatal("stats for an impossible pid succeeded")
	}
}

func TestDescendants(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	kids, err := Descendants(os.Getpid())
	if err != nil {
		t.Fatalf("Descendants: %v", err)
	}
	for _, pid := range kids {
		if pid == cmd.Process.Pid {
			return
		}
	}
	t.Fatalf("Descendants(self) = %v, missing %d", kids, cmd.Process.Pid)
}

func TestReadTree(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	tree, err := ReadTree(os.Getpid())
	if err != nil {
		t.Fatalf("ReadTree: %v", err)
	}
	if len(tree) == 0 || tree[0].PID != os.Getpid() || tree[0].Name == "" {
		t.Fatalf("tree root = %+v", tree)
	}
	for _, s := range tree[1:] {
		if s.PID == cmd.Process.Pid {
			return
		}
	}
	t.Fatalf("ReadTree(self) pids = %v, missing %d", tree, cmd.Process.Pid)
}

type resolverFunc func(string) (string, bool)

func (f resolverFunc) Resolve(name string) (string, bool) { return f(name) }

func count(ks []DiagKind, k DiagKind) int {
	n := 0
	for _, x := range ks {
		if x == k {
			n++
		}
	}
	return n
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// waitGone fails unless pid disappears within a few seconds.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pid %d survived the group kill", pid)
}

func TestStripNoise(t *testing.T) {
	in := "zle: x\nRunning as unit: run-1.scope\nScope command line contains environment variable $X\nreal\n"
	if got := string(StripNoise([]byte(in))); got != "real" {
		t.Fatalf("StripNoise = %q", got)
	}
}

func TestPackageFindSeesPathChanges(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if _, err := Find("latetool"); err == nil {
		t.Fatal("found tool before it existed")
	}
	want := writeScript(t, dir, "latetool", "exit 0")
	if p, err := Find("latetool"); err != nil || p != want {
		t.Fatalf("Find = %q %v", p, err)
	}
	other := t.TempDir()
	moved := writeScript(t, other, "latetool", "exit 0")
	t.Setenv("PATH", other)
	if p, err := Find("latetool"); err != nil || p != moved {
		t.Fatalf("Find after PATH change = %q %v", p, err)
	}
}

func TestDefaultShellChildEnvExported(t *testing.T) {
	t.Setenv("SHELL", writeScript(t, t.TempDir(), "sh", `echo "m=$`+DefaultShellChildEnv+`"`))
	res, err := Cmd("nope-not-a-tool-q").ShellFallback().Run(context.Background())
	if err != nil || string(res.Stdout) != "m=1\n" {
		t.Fatalf("fallback = %q %v", res.Stdout, err)
	}
}

func TestInjectedEnvSource(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := writeScript(t, toolDir, "envtool", `echo "$EXTRA"`)
	shell := writeScript(t, dir, "mysh", `exec /bin/sh "$@"`)
	rc := NewRunContext("",
		WithEnvSource(
			func(key string) string {
				if key == "SHELL" {
					return shell
				}
				return ""
			},
			func() []string { return []string{"PATH=" + toolDir} },
		),
		WithEnv("EXTRA=1"),
	)
	p, err := rc.Find("envtool")
	if err != nil || p != tool {
		t.Fatalf("Find via injected env = %q %v", p, err)
	}
	res, err := rc.Cmd("envtool").ShellFallback().Run(context.Background())
	if err != nil || string(res.Stdout) != "1\n" {
		t.Fatalf("fallback via injected env = %q %v", res.Stdout, err)
	}
}

func TestEnvOptionOrderIndependent(t *testing.T) {
	src := WithEnvSource(nil, func() []string { return []string{"PATH=/usr/bin:/bin", "BASE=b"} })
	for name, rc := range map[string]*RunContext{
		"source first": NewRunContext("", src, WithEnv("EXTRA=1")),
		"env first":    NewRunContext("", WithEnv("EXTRA=1"), src),
		"derived":      NewRunContext("", src).Derive(WithEnv("EXTRA=1")),
	} {
		res, err := rc.CmdPath("/bin/sh", "-c", `echo "$BASE$EXTRA"`).Run(context.Background())
		if err != nil || string(res.Stdout) != "b1\n" {
			t.Errorf("%s: got %q %v", name, res.Stdout, err)
		}
	}
}

func TestWaitMapsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	proc, err := Cmd("sh", "-c", `trap '' TERM; while :; do sleep 0.05; done`).Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, cancel)
	res, err := proc.Wait()
	if res == nil {
		t.Fatal("Wait returned no result")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait = %v, want error unwrapping to context.Canceled", err)
	}
}
