// Package capbuf is a head-keeping output buffer capped at 16 MiB.
package capbuf

import (
	"bytes"
	"io"
)

// maxBufferBytes caps captured output at 16 MiB.
const maxBufferBytes = 16 << 20

// Buffer captures subprocess output, keeping the first 16 MiB and
// dropping the rest. It is written by exec's copy goroutine and read only
// after Wait has joined it, so it needs no locking.
type Buffer struct {
	buf       bytes.Buffer
	truncated bool
}

// Write keeps what fits under the cap and always reports full success so
// exec's copy loop never aborts on overflow.
func (b *Buffer) Write(p []byte) (int, error) {
	room := maxBufferBytes - b.buf.Len()
	if room < len(p) {
		b.truncated = true
	}
	if room > 0 {
		b.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// ReadFrom routes io.Copy's fast path through the capped Write.
func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{b}, r)
}

// Bytes returns the captured bytes.
func (b *Buffer) Bytes() []byte { return b.buf.Bytes() }

// Len returns the number of captured bytes.
func (b *Buffer) Len() int { return b.buf.Len() }

// Truncated reports whether any byte was dropped.
func (b *Buffer) Truncated() bool { return b.truncated }
