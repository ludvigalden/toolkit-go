package linestream

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestPumpSplitsLines(t *testing.T) {
	lines, raw, done := make(chan string, 8), make(chan []byte, 8), make(chan struct{})
	go Pump(context.Background(), io.NopCloser(strings.NewReader("a\nb\npartial")), lines, raw, done)
	<-done
	var got []string
	for l := range lines {
		got = append(got, l)
	}
	if strings.Join(got, ",") != "a,b,partial" {
		t.Fatalf("lines = %q", got)
	}
}

func TestDecodeAndNonBlank(t *testing.T) {
	if _, obj, ok := Decode(`{"a":1}`); !ok || obj["a"] != float64(1) {
		t.Fatalf("object decode = %v %v", obj, ok)
	}
	if _, obj, ok := Decode(`[1]`); !ok || obj != nil {
		t.Fatal("array decoded as object")
	}
	if _, _, ok := Decode("plain"); ok {
		t.Fatal("plain text decoded")
	}
	if got := NonBlank([]byte(" x \n\n y\n")); len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Fatalf("NonBlank = %q", got)
	}
}
