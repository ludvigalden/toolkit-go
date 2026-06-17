package svcenv

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestCapture(t *testing.T) {
	got, err := Capture(env(map[string]string{
		"HOME":           "/home/u",
		"XDG_CACHE_HOME": "/c",
		"PATH":           "/ambient/should/not/leak",
	}), "/opt/tool/bin/tool", "/usr/bin:/bin")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"HOME":            "/home/u",
		"XDG_CACHE_HOME":  "/c",
		"XDG_CONFIG_HOME": "/home/u/.config",
		"XDG_DATA_HOME":   "/home/u/.local/share",
		"XDG_STATE_HOME":  "/home/u/.local/state",
		"PATH":            "/opt/tool/bin:/usr/bin:/bin",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestCaptureRejects(t *testing.T) {
	for name, m := range map[string]map[string]string{
		"no home":       {},
		"relative home": {"HOME": "home"},
		"newline":       {"HOME": "/h", "XDG_DATA_HOME": "/d\nX=1"},
	} {
		if _, err := Capture(env(m), "", "/bin"); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestPATH(t *testing.T) {
	if got := PATH("", "/bin"); got != "/bin" {
		t.Errorf("PATH(\"\") = %q", got)
	}
}
