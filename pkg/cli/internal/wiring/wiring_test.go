package wiring

import "testing"

func TestEnvPrefix(t *testing.T) {
	for _, c := range [][3]string{{"my-tool", "", "MY_TOOL_"}, {"x", "CUSTOM_", "CUSTOM_"}, {"x", "CUSTOM", "CUSTOM_"}} {
		if got := EnvPrefix(c[0], c[1]); got != c[2] {
			t.Errorf("EnvPrefix(%q,%q) = %q", c[0], c[1], got)
		}
	}
}

func TestSessionID(t *testing.T) {
	if id := SessionID(); len(id) != 8 {
		t.Errorf("SessionID = %q", id)
	}
}
