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
