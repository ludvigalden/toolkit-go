package cascade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkUpStopsAtHome(t *testing.T) {
	home := "/home/u"
	if got := WalkUp("/home/u/a/b", home); strings.Join(got, ",") != "/home/u,/home/u/a,/home/u/a/b" {
		t.Errorf("inside: %v", got)
	}
	if got := WalkUp("/srv/x", home); len(got) != 1 || got[0] != "/srv/x" {
		t.Errorf("outside: %v", got)
	}
	if got := WalkUp("/home/user2", home); len(got) != 1 {
		t.Errorf("sibling prefix: %v", got)
	}
}

func TestBuiltinsAndNames(t *testing.T) {
	d := WithBuiltins(map[string]any{"color": "never", "x.y": 1})
	if d["color"] != "never" || d["stdout_format"] != "text" || d["x.y"] != 1 {
		t.Errorf("defaults %v", d)
	}
	if got := LocalNamesFor("demo", nil); strings.Join(got, ",") != "demo.yaml,.demo.yaml" {
		t.Errorf("names %v", got)
	}
	if Profile("", "DEMO_", func(k string) string { return map[string]string{"DEMO_PROFILE": "p"}[k] }) != "p" {
		t.Error("profile from env")
	}
}

func TestEnvPresenceSemantics(t *testing.T) {
	user := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(user, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("config.yaml", "demo:\n  dir: file\n")
	layers := func(getenv func(string) (string, bool)) Layers {
		return Layers{
			Name: "demo", Prefix: "DEMO_",
			Defaults: map[string]any{"demo.dir": "cfg"},
			UserDir:  user, Getenv: getenv,
		}
	}
	k, err := layers(func(string) (string, bool) { return "", false }).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := k.String("demo.dir"); got != "file" {
		t.Errorf("absent env did not fall through to config: %q", got)
	}
	write(".env", "DEMO_DIR=dotenv\n")
	k, err = layers(func(string) (string, bool) { return "", false }).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := k.String("demo.dir"); got != "dotenv" {
		t.Errorf("absent env did not fall through to dotenv: %q", got)
	}
	k, err = layers(func(string) (string, bool) { return "", true }).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := k.String("demo.dir"); got != "" {
		t.Errorf("set-but-empty did not clear config and dotenv: %q", got)
	}
	t.Setenv("DEMO_DIR", "proc")
	k, err = layers(nil).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := k.String("demo.dir"); got != "proc" {
		t.Errorf("nil Getenv did not read the process env: %q", got)
	}
	t.Setenv("DEMO_DIR", "")
	k, err = layers(nil).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := k.String("demo.dir"); got != "" {
		t.Errorf("process env set-but-empty lost to dotenv: %q", got)
	}
}

func TestEnvFallbacks(t *testing.T) {
	f := FallbacksWithBuiltins(map[string][]string{"session": {"OTHER_SESSION"}})
	if strings.Join(f["stdout_format"], ",") != "STDOUT_FORMAT" || f["session"][0] != "OTHER_SESSION" {
		t.Errorf("fallbacks %v", f)
	}
	if f := FallbacksWithBuiltins(map[string][]string{"stdout_format": nil}); len(f["stdout_format"]) != 0 {
		t.Errorf("nil entry kept the built-in: %v", f)
	}
	user := t.TempDir()
	if err := os.WriteFile(filepath.Join(user, ".env"), []byte("A_FMT=dotenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	load := func(env map[string]string) string {
		t.Helper()
		k, err := Layers{
			Name: "demo", Prefix: "DEMO_", UserDir: user,
			Defaults:     map[string]any{"fmt": "default"},
			EnvFallbacks: map[string][]string{"fmt": {"A_FMT", "B_FMT"}},
			Getenv: func(key string) (string, bool) {
				v, ok := env[key]
				return v, ok
			},
		}.Load()
		if err != nil {
			t.Fatal(err)
		}
		return k.String("fmt")
	}
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"DEMO_FMT": "prefixed", "A_FMT": "a", "B_FMT": "b"}, "prefixed"},
		{map[string]string{"A_FMT": "a", "B_FMT": "b"}, "a"},
		{map[string]string{"B_FMT": "b"}, "dotenv"},
		{map[string]string{}, "dotenv"},
	} {
		if got := load(c.env); got != c.want {
			t.Errorf("env %v: %q, want %q", c.env, got, c.want)
		}
	}
}
