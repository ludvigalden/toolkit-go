package shell

import (
	"os/exec"
	"testing"
)

func TestQuote(t *testing.T) {
	out, err := exec.Command("sh", "-c", "printf %s "+Quote("it's a $x")).Output()
	if err != nil || string(out) != "it's a $x" {
		t.Fatalf("quoted = %q %v", out, err)
	}
}

func TestLine(t *testing.T) {
	if got := Line("echo", []string{"a b", "c"}); got != "echo 'a b' 'c'" {
		t.Fatalf("Line = %q", got)
	}
}
