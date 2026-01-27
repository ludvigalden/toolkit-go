package xdg

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveDefaults(t *testing.T) {
	d, err := Resolve(env(map[string]string{"HOME": "/home/u"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Dirs{
		ConfigHome: "/home/u/.config",
		CacheHome:  "/home/u/.cache",
		StateHome:  "/home/u/.local/state",
		DataHome:   "/home/u/.local/share",
		ConfigDirs: []string{"/etc/xdg"},
	}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("got %+v\nwant %+v", d, want)
	}
}

func TestResolveOverrides(t *testing.T) {
	d, err := Resolve(env(map[string]string{
		"HOME":              "/home/u",
		"XDG_CONFIG_HOME":   "/cfg",
		"XDG_CACHE_HOME":    "/cache/",
		"XDG_STATE_HOME":    "/st",
		"XDG_DATA_HOME":     "/data",
		"XDG_CONFIG_DIRS":   "/a:/b",
		"XDG_RUNTIME_DIR":   "/run/1",
		"XDG_EXTRA_IGNORED": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if d.ConfigHome != "/cfg" || d.CacheHome != "/cache" || d.StateHome != "/st" ||
		d.DataHome != "/data" || d.RuntimeDir != "/run/1" {
		t.Errorf("dirs = %+v", d)
	}
	if !reflect.DeepEqual(d.ConfigDirs, []string{"/a", "/b"}) {
		t.Errorf("ConfigDirs = %v", d.ConfigDirs)
	}
}

func TestResolveIgnoresRelativeAndEmpty(t *testing.T) {
	d, err := Resolve(env(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "rel",
		"XDG_RUNTIME_DIR": "",
		"XDG_CONFIG_DIRS": "x:./y:/z",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if d.ConfigHome != "/home/u/.config" || d.RuntimeDir != "" {
		t.Errorf("dirs = %+v", d)
	}
	if !reflect.DeepEqual(d.ConfigDirs, []string{"/z"}) {
		t.Errorf("ConfigDirs = %v", d.ConfigDirs)
	}
	if _, err := Resolve(env(map[string]string{})); !errors.Is(err, ErrNoHome) {
		t.Errorf("err = %v, want ErrNoHome", err)
	}
	if _, err := Resolve(env(map[string]string{"HOME": "rel"})); !errors.Is(err, ErrNoHome) {
		t.Errorf("relative HOME err = %v, want ErrNoHome", err)
	}
}

func TestRuntimeDirFor(t *testing.T) {
	base := t.TempDir()
	t.Setenv("TMPDIR", base)
	rd, err := RuntimeDirFor("tool", env(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "tool-"+strconv.Itoa(os.Getuid()))
	if rd != want {
		t.Errorf("rd = %s, want %s", rd, want)
	}
	info, err := os.Stat(rd)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("mode = %o, want 700", info.Mode().Perm())
	}
	// preferred runtime dir
	pref := filepath.Join(base, "rt")
	rd, err = RuntimeDirFor("tool", env(map[string]string{"XDG_RUNTIME_DIR": pref}))
	if err != nil || rd != filepath.Join(pref, "tool") {
		t.Errorf("rd = %s, %v", rd, err)
	}
	if info, _ := os.Stat(filepath.Join(pref, "tool")); info == nil || info.Mode().Perm() != 0o700 {
		t.Errorf("preferred dir mode wrong: %v", info)
	}
	// invalid app names
	for _, app := range []string{"", ".", "..", "a/b"} {
		if _, err := RuntimeDirFor(app, env(nil)); err == nil {
			t.Errorf("app %q accepted", app)
		}
	}
	// invalid XDG_RUNTIME_DIR falls back
	rd, err = RuntimeDirFor("tool", env(map[string]string{"XDG_RUNTIME_DIR": "rel"}))
	if err != nil || rd != want {
		t.Errorf("relative runtime dir: %s, %v", rd, err)
	}
}

func TestFallbackRejectsBadDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("TMPDIR", base)
	// pre-create with wrong mode
	loose := filepath.Join(base, "tool-"+strconv.Itoa(os.Getuid()))
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := FallbackRuntimeDir("tool"); err == nil {
		t.Error("loose fallback accepted")
	}
	// a symlink to elsewhere is refused
	if err := os.RemoveAll(loose); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "victim")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, loose); err != nil {
		t.Fatal(err)
	}
	if _, err := FallbackRuntimeDir("tool"); err == nil {
		t.Error("symlink fallback accepted")
	}
}

func TestRuntimeDirsScan(t *testing.T) {
	base := t.TempDir()
	t.Setenv("TMPDIR", base)
	pref := filepath.Join(base, "rt")
	got := RuntimeDirs("nosuch", env(map[string]string{}))
	if len(got) != 0 {
		t.Errorf("RuntimeDirs(nosuch) = %v", got)
	}
	if _, err := RuntimeDirFor("app", env(map[string]string{"XDG_RUNTIME_DIR": pref})); err != nil {
		t.Fatal(err)
	}
	got = RuntimeDirs("app", env(map[string]string{"XDG_RUNTIME_DIR": pref}))
	if len(got) != 1 || got[0] != filepath.Join(pref, "app") {
		t.Errorf("RuntimeDirs = %v", got)
	}
}

func TestEnsureDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "a", "b")
	got, err := EnsureDir(dir, 0o700)
	if err != nil || got != nil {
		t.Fatalf("create: %v, %v", got, err)
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	// matching mode: no drift
	if got, err := EnsureDir(dir, 0o700); err != nil || got != nil {
		t.Errorf("match: %v, %v", got, err)
	}
	// mismatch: drift reported, dir untouched
	if got, err := EnsureDir(dir, 0o755); err != nil || got == nil {
		t.Errorf("drift: %v, %v", got, err)
	} else if got.Want != 0o755 || got.Actual != 0o700 {
		t.Errorf("drift = %+v", *got)
	}
	info, _ = os.Stat(dir)
	if info.Mode().Perm() != 0o700 {
		t.Error("EnsureDir chmod'ed on mismatch")
	}
	// repair
	if err := RepairDir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, _ := EnsureDir(dir, 0o755); got != nil {
		t.Errorf("still drifting: %v", got)
	}
	// file where a dir belongs
	f := filepath.Join(base, "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDir(f, 0o700); !errors.Is(err, ErrNotDir) {
		t.Errorf("file err = %v", err)
	}
	if err := RepairDir(f, 0o700); !errors.Is(err, ErrNotDir) {
		t.Errorf("repair file err = %v", err)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	base := t.TempDir()
	if err := WriteFileAtomic(base, "c.json", []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(base, "c.json"))
	if err != nil || string(b) != `{"a":1}` {
		t.Fatalf("read: %q, %v", b, err)
	}
	info, _ := os.Stat(filepath.Join(base, "c.json"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o", info.Mode().Perm())
	}
	// replace an existing file; leftovers cleaned
	if err := WriteFileAtomic(base, "c.json", []byte(`{"a":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(base, "c.json"))
	if string(b) != `{"a":2}` {
		t.Errorf("replace: %q", b)
	}
	entries, _ := os.ReadDir(base)
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("temp files left: %v", names)
	}
	for _, name := range []string{"", ".", "..", "a/b"} {
		if err := WriteFileAtomic(base, name, nil, 0o600); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	// failing write (missing dir) leaves no panic and reports an error
	if err := WriteFileAtomic(filepath.Join(base, "nope"), "x", nil, 0o600); err == nil {
		t.Error("missing dir accepted")
	}
}
