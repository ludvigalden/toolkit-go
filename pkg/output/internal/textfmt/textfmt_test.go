package textfmt

import (
	"testing"
	"time"
)

func TestLine(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := Line(at, "error", true, "E1", "boom", false); got != "03:04:05 FAIL [E1] boom" {
		t.Errorf("failure line = %q", got)
	}
	if got := StripANSI(Line(at, "warn", false, "", "careful", true)); got != "03:04:05 WARN careful" {
		t.Errorf("colored line = %q", got)
	}
}

func TestColorEnabled(t *testing.T) {
	env := map[string]string{"NO_COLOR": "1"}
	if ColorEnabled("always", nil, func(k string) string { return env[k] }) {
		t.Error("NO_COLOR ignored")
	}
	env = map[string]string{"CLICOLOR_FORCE": "1"}
	if !ColorEnabled("auto", nil, func(k string) string { return env[k] }) {
		t.Error("CLICOLOR_FORCE ignored")
	}
}
