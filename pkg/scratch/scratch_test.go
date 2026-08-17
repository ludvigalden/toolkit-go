package scratch_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/scratch"
)

func newEnv(t *testing.T, opts ...scratch.Option) *scratch.Env {
	t.Helper()
	opts = append([]scratch.Option{scratch.WithRootParent(t.TempDir())}, opts...)
	e, err := scratch.New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = e.Cleanup() })
	return e
}

func TestChildEnvFenced(t *testing.T) {
	t.Setenv("HOME", "/definitely/not/the/scratch/home")
	e := newEnv(t)
	env := map[string]string{}
	for _, entry := range e.ChildEnv() {
		k, v, _ := strings.Cut(entry, "=")
		env[k] = v
	}
	core := map[string]string{
		"HOME":            "home",
		"TMPDIR":          "tmp",
		"XDG_CONFIG_HOME": "home/.config",
		"XDG_CACHE_HOME":  "home/.cache",
		"XDG_DATA_HOME":   "home/.local/share",
		"XDG_STATE_HOME":  "home/.local/state",
	}
	for key, rel := range core {
		want := filepath.Join(e.Root(), rel)
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}
	if env["HOME"] == "/definitely/not/the/scratch/home" {
		t.Error("host HOME leaked into ChildEnv")
	}
	for _, dir := range core {
		if fi, err := os.Stat(filepath.Join(e.Root(), dir)); err != nil || !fi.IsDir() {
			t.Errorf("fenced dir %s missing: %v", dir, err)
		}
	}
	if _, err := os.Stat(e.Dir()); err != nil {
		t.Errorf("working dir missing: %v", err)
	}
}

func TestChildEnvExtraFencedVar(t *testing.T) {
	e := newEnv(t, scratch.WithFencedVar("TOOL_STATE", "state/tool"))
	for _, entry := range e.ChildEnv() {
		if entry == "TOOL_STATE="+filepath.Join(e.Root(), "state/tool") {
			return
		}
	}
	t.Error("TOOL_STATE missing or wrong in ChildEnv")
	if _, err := os.Stat(filepath.Join(e.Root(), "state/tool")); err != nil {
		t.Errorf("extra fenced dir missing: %v", err)
	}
}

func TestResolveRejectsEscapes(t *testing.T) {
	e := newEnv(t)
	for _, rel := range []string{"", ".", "..", "../outside", "/abs"} {
		if _, err := e.Resolve(rel); !errors.Is(err, scratch.ErrTraversal) {
			t.Errorf("Resolve(%q) err = %v, want ErrTraversal", rel, err)
		}
	}
	if _, err := e.Resolve("nested/file.txt"); err != nil {
		t.Errorf("Resolve valid: %v", err)
	}
}

func TestWriteRejectsSymlinkEscape(t *testing.T) {
	e := newEnv(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.Root(), "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resolve("escape/file"); !errors.Is(err, scratch.ErrSymlinkEscape) {
		t.Errorf("Resolve through symlink err = %v, want ErrSymlinkEscape", err)
	}
	if err := e.WriteFile("escape/file", []byte("x"), 0); !errors.Is(err, scratch.ErrSymlinkEscape) {
		t.Errorf("WriteFile through symlink err = %v, want ErrSymlinkEscape", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file")); !errors.Is(err, os.ErrNotExist) {
		t.Error("write escaped the root through the symlink")
	}
}

func TestWriteAndMkdir(t *testing.T) {
	e := newEnv(t)
	if err := e.Mkdir("a/b"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := e.WriteFile("a/b/file.txt", []byte("content"), 0); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(e.Root(), "a/b/file.txt"))
	if err != nil || string(got) != "content" {
		t.Errorf("read back %q, err %v", got, err)
	}
}

func TestSeedHostFixture(t *testing.T) {
	e := newEnv(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "data.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(src, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "n.txt"), []byte("n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.Seed("scenario", src); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	for rel, want := range map[string]string{
		"scenario/data.txt":  "fixture",
		"scenario/sub/n.txt": "n",
	} {
		got, err := os.ReadFile(filepath.Join(e.Root(), rel))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, err %v", rel, got, err)
		}
	}
	if err := e.Seed("..", src); !errors.Is(err, scratch.ErrTraversal) {
		t.Errorf("Seed escape err = %v, want ErrTraversal", err)
	}
}

func TestSeedFS(t *testing.T) {
	e := newEnv(t)
	fixtures := fstest.MapFS{
		"root/app.yaml":  &fstest.MapFile{Data: []byte("cfg")},
		"root/dir/n.txt": &fstest.MapFile{Data: []byte("n")},
	}
	if err := e.SeedFS("mounted", fixtures, "root"); err != nil {
		t.Fatalf("SeedFS: %v", err)
	}
	for rel, want := range map[string]string{
		"mounted/app.yaml":  "cfg",
		"mounted/dir/n.txt": "n",
	} {
		got, err := fs.ReadFile(os.DirFS(e.Root()), rel)
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, err %v", rel, got, err)
		}
	}
	if err := e.SeedFS("bad", fixtures, "../escape"); err == nil {
		t.Error("SeedFS accepted an invalid fs path")
	}
}

func TestSeedFSOverEmbedMode(t *testing.T) {
	// embed.FS reports 0444; a seeded file must stay writable so a
	// later WriteFile to the same rel succeeds.
	e := newEnv(t)
	fixtures := fstest.MapFS{
		"root/app.yaml": &fstest.MapFile{Data: []byte("cfg"), Mode: 0o444},
	}
	if err := e.SeedFS("mounted", fixtures, "root"); err != nil {
		t.Fatalf("SeedFS: %v", err)
	}
	if err := e.WriteFile("mounted/app.yaml", []byte("rewritten"), 0); err != nil {
		t.Fatalf("WriteFile after seed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(e.Root(), "mounted/app.yaml"))
	if err != nil || string(got) != "rewritten" {
		t.Errorf("read back %q, err %v", got, err)
	}
}

func TestCleanupRemovesRoot(t *testing.T) {
	parent := t.TempDir()
	e, err := scratch.New(scratch.WithRootParent(parent))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(e.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Error("root survived Cleanup")
	}
	if err := e.Cleanup(); err != nil {
		t.Errorf("second Cleanup not idempotent: %v", err)
	}
}

func TestCleanupRetainOnFailure(t *testing.T) {
	parent := t.TempDir()
	var notices bytes.Buffer
	e, err := scratch.New(
		scratch.WithRootParent(parent),
		scratch.WithRetainOnFailure(),
		scratch.WithStderr(&notices),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(e.Root()) })
	e.Fail()
	if err := e.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(e.Root()); err != nil {
		t.Error("root removed despite retained failure")
	}
	if err := e.Cleanup(); err != nil {
		t.Fatalf("second Cleanup: %v", err)
	}
	if got := strings.Count(notices.String(), "retained"); got != 1 {
		t.Errorf("retained notice printed %d times, want 1: %q", got, notices.String())
	}
}

func TestSetupFailureRemovesPartialRoot(t *testing.T) {
	parent := t.TempDir()
	_, err := scratch.New(
		scratch.WithRootParent(parent),
		scratch.WithFencedVar("HOME", "hijack"),
	)
	if err == nil {
		t.Fatal("redefining a core fenced variable should fail")
	}
	entries, readErr := os.ReadDir(parent)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("partial roots survived setup failure: %v", entries)
	}
}

func TestSetupFailureRetained(t *testing.T) {
	parent := t.TempDir()
	var notices bytes.Buffer
	e, err := scratch.New(
		scratch.WithRootParent(parent),
		scratch.WithRetainOnFailure(),
		scratch.WithStderr(&notices),
		scratch.WithFencedVar("HOME", "hijack"),
	)
	if err == nil {
		t.Fatal("redefining a core fenced variable should fail")
	}
	if e != nil {
		t.Error("New returned an Env despite setup failure")
	}
	if !strings.Contains(notices.String(), "retained") {
		t.Errorf("retained notice missing: %q", notices.String())
	}
	entries, readErr := os.ReadDir(parent)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("retained partial roots = %d, want 1", len(entries))
	}
	_ = os.RemoveAll(filepath.Join(parent, entries[0].Name()))
}

func TestCommandContextFencedRun(t *testing.T) {
	e := newEnv(t)
	out, err := e.CommandContext(context.Background(), "sh", "-c",
		"pwd && printf '%s\n' \"$HOME\" && printf '%s\n' \"$TMPDIR\"").Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{e.Dir(), filepath.Join(e.Root(), "home"), filepath.Join(e.Root(), "tmp")}
	if len(lines) != len(want) {
		t.Fatalf("output lines = %q, want %d lines", lines, len(want))
	}
	for i, w := range want {
		if realpath(t, lines[i]) != realpath(t, w) {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestCommandContextGroupKillOnCancel(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := e.CommandContext(ctx, "sh", "-c",
		"echo $$ > pids\nsleep 30 &\necho $! >> pids\nwait")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	var pids []int
	for {
		data, err := os.ReadFile(filepath.Join(e.Dir(), "pids"))
		if err == nil {
			for _, f := range strings.Fields(string(data)) {
				if p, err := strconv.Atoi(f); err == nil {
					pids = append(pids, p)
				}
			}
			if len(pids) == 2 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("child never reported its pids")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-waitErr:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after cancellation")
	}
	for _, pid := range pids {
		if !waitDead(pid, 5*time.Second) {
			t.Errorf("process %d still alive after group kill", pid)
		}
	}
}

// realpath resolves symlinks because pwd reports the physical path while
// the scratch root may sit below a symlinked temporary directory.
func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("resolve %q: %v", p, err)
	}
	return r
}

// waitDead polls until pid is gone. A killed grandchild is reparented
// and may linger as a zombie until reaped, which kill(pid, 0) still
// reports as present, so a Linux zombie state counts as dead.
func waitDead(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil || isZombie(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func isZombie(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	i := bytes.LastIndexByte(data, ')')
	return i >= 0 && i+2 < len(data) && data[i+2] == 'Z'
}
