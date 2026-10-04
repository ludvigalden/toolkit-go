package terminal

import (
	"bytes"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/output"
)

func TestNonTerminalFallsBackToText(t *testing.T) {
	var stdout, stderr bytes.Buffer
	o, err := output.New(&stdout, &stderr, output.WithMode(Mode), Option())
	if err != nil {
		t.Fatal(err)
	}
	o.Result("payload")
	o.Warn("w")
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "payload\n" || !bytes.Contains(stderr.Bytes(), []byte("WARN w")) {
		t.Errorf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPercentOf(t *testing.T) {
	for in, want := range map[string]float64{"42% done": 0.42, "working": 0.5, "150%": 1, "x 7.5%": 0.075} {
		if got := percentOf(in); got != want {
			t.Errorf("percentOf(%q) = %v, want %v", in, got, want)
		}
	}
}
