package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestPassHeaderRoundTrip(t *testing.T) {
	h := PassHeader{Argon: Argon{Time: 3, Memory: 9, Threads: 2}}
	h.Salt[0], h.Prefix[7] = 0xaa, 0xbb
	var b bytes.Buffer
	if err := WritePass(&b, h); err != nil {
		t.Fatal(err)
	}
	if b.Len() != passHeaderSize || !IsPass(b.Bytes()) || IsRaw(b.Bytes()) {
		t.Fatalf("header %x", b.Bytes())
	}
	got, err := ReadPass(&b)
	if err != nil || got != h {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestReadPassRejects(t *testing.T) {
	var good bytes.Buffer
	_ = WritePass(&good, PassHeader{Argon: Argon{Time: 1, Memory: 1, Threads: 1}})
	mutate := func(i int, v byte) []byte {
		b := bytes.Clone(good.Bytes())
		b[i] = v
		return b
	}
	tests := []struct {
		name string
		b    []byte
		want error
	}{
		{"bad magic", mutate(0, 'x'), ErrBadFormat},
		{"bad version", mutate(MagicSize, 2), ErrUnsupportedVersion},
		{"bad kdf", mutate(MagicSize+1, 7), ErrBadFormat},
		{"zero threads", mutate(MagicSize+2+8, 0), ErrBadFormat},
		{"short", good.Bytes()[:10], ErrBadFormat},
	}
	for _, tt := range tests {
		if _, err := ReadPass(bytes.NewReader(tt.b)); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

// TestReadPassRejectsOversizedArgon crafts headers whose argon2id cost
// exceeds the header limits and asserts ReadPass refuses them before
// any key derivation would run.
func TestReadPassRejectsOversizedArgon(t *testing.T) {
	var good bytes.Buffer
	_ = WritePass(&good, PassHeader{Argon: Argon{Time: 1, Memory: 1, Threads: 1}})
	setCost := func(time, memory uint32, threads byte) []byte {
		b := bytes.Clone(good.Bytes())
		binary.BigEndian.PutUint32(b[MagicSize+2:], time)
		binary.BigEndian.PutUint32(b[MagicSize+6:], memory)
		b[MagicSize+2+8] = threads
		return b
	}
	tests := []struct {
		name string
		b    []byte
		want string
	}{
		{"memory", setCost(1, MaxArgonMemory+1, 1), "memory"},
		{"time", setCost(MaxArgonTime+1, 1, 1), "time"},
		{"threads", setCost(1, 1, MaxArgonThreads+1), "threads"},
	}
	for _, tt := range tests {
		_, err := ReadPass(bytes.NewReader(tt.b))
		if !errors.Is(err, ErrBadFormat) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want rejection naming %q", tt.name, err, tt.want)
		}
	}
	if _, err := ReadPass(bytes.NewReader(setCost(MaxArgonTime, MaxArgonMemory, MaxArgonThreads))); err != nil {
		t.Errorf("costs at the limits rejected: %v", err)
	}
}

func TestRawHeaderRoundTrip(t *testing.T) {
	var p [PrefixSize]byte
	p[3] = 5
	var b bytes.Buffer
	if err := WriteRaw(&b, p); err != nil {
		t.Fatal(err)
	}
	if b.Len() != rawHeaderSize || !IsRaw(b.Bytes()) {
		t.Fatalf("header %x", b.Bytes())
	}
	got, err := ReadRaw(&b)
	if err != nil || got != p {
		t.Fatalf("got %x, %v", got, err)
	}
}

func TestChunkBounds(t *testing.T) {
	aead, err := NewAEAD(make([]byte, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	var p [PrefixSize]byte
	for name, length := range map[string][]byte{
		"under tag": {0, 0, 0, 1},
		"oversized": {0xff, 0xff, 0xff, 0xff},
	} {
		if err := Open(&bytes.Buffer{}, bytes.NewReader(length), aead, p); !errors.Is(err, ErrBadFormat) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
