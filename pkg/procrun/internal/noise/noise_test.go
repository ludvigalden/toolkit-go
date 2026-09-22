package noise

import "testing"

func TestStrip(t *testing.T) {
	in := "zle: can't change option: zle\nRunning as unit: run-1.scope\nreal error\n\n"
	if got := string(Strip([]byte(in))); got != "real error" {
		t.Fatalf("Strip = %q", got)
	}
	if Strip([]byte("Running as unit: x\n")) != nil {
		t.Fatal("all-noise input kept lines")
	}
}
