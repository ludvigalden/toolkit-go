package linewire

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

func TestReadLineAndWrap(t *testing.T) {
	br := bufio.NewReader(strings.NewReader("hello\npayload"))
	if line, err := ReadLine(br, 64); err != nil || string(line) != "hello" {
		t.Fatalf("ReadLine = %q %v", line, err)
	}
	a, b := net.Pipe()
	defer func() { _, _ = a.Close(), b.Close() }()
	got := make([]byte, 7)
	if _, err := Wrap(a, br).Read(got); err != nil || string(got) != "payload" {
		t.Fatalf("buffered bytes = %q %v", got, err)
	}
	if _, err := ReadLine(bufio.NewReader(strings.NewReader(strings.Repeat("x", 100)+"\n")), 10); err == nil {
		t.Fatal("oversized line accepted")
	}
}
