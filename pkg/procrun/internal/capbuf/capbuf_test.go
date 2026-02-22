package capbuf

import (
	"bytes"
	"io"
	"testing"
)

func TestBufferCaps(t *testing.T) {
	var b Buffer
	big := bytes.Repeat([]byte("x"), maxBufferBytes-3)
	if n, err := b.Write(big); n != len(big) || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if b.Truncated() {
		t.Fatal("truncated before the cap")
	}
	if n, _ := b.Write([]byte("abcdef")); n != 6 {
		t.Fatalf("overflow write reported %d", n)
	}
	if !b.Truncated() || b.Len() != maxBufferBytes {
		t.Fatalf("len=%d truncated=%v", b.Len(), b.Truncated())
	}
	if got := b.Bytes()[b.Len()-3:]; string(got) != "abc" {
		t.Fatalf("tail = %q", got)
	}
}

func TestBufferReadFromCaps(t *testing.T) {
	var b Buffer
	src := io.LimitReader(zeroReader{}, maxBufferBytes+1024)
	n, err := io.Copy(&b, src)
	if err != nil || n != maxBufferBytes+1024 {
		t.Fatalf("Copy = %d, %v", n, err)
	}
	if !b.Truncated() || b.Len() != maxBufferBytes {
		t.Fatalf("len=%d truncated=%v", b.Len(), b.Truncated())
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
