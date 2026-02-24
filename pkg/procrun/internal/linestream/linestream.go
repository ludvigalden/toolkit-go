// Package linestream splits subprocess stdout into raw chunks and lines
// for streaming readers and classifies lines as JSON or plain text.
package linestream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// Pump copies r into raw chunks and lines until EOF, then closes lines,
// raw and done. Sends never block, so a slow consumer loses items; ctx
// ending closes r so an unwaited process cannot leak the goroutine.
func Pump(ctx context.Context, r io.ReadCloser, lines chan<- string, raw chan<- []byte, done chan<- struct{}) {
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = r.Close() //nolint:errcheck // unblocks the read below
		case <-stop:
		}
	}()
	defer func() {
		close(stop)
		_ = r.Close() //nolint:errcheck // reader done
		close(lines)
		close(raw)
		close(done)
	}()
	buf := make([]byte, 4096)
	var partial []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			trySend(raw, chunk)
			partial = append(partial, chunk...)
			for {
				i := bytes.IndexByte(partial, '\n')
				if i < 0 {
					break
				}
				trySend(lines, string(partial[:i]))
				partial = partial[i+1:]
			}
		}
		if err != nil {
			if len(partial) > 0 {
				trySend(lines, string(partial))
			}
			return
		}
	}
}

// trySend delivers v without blocking a slow consumer.
func trySend[T any](ch chan<- T, v T) {
	select {
	case ch <- v:
	default:
	}
}

// Decode parses line as JSON. ok is false for non-JSON lines; obj is set
// only when the value is an object.
func Decode(line string) (v any, obj map[string]any, ok bool) {
	if json.Unmarshal([]byte(line), &v) != nil {
		return nil, nil, false
	}
	obj, _ = v.(map[string]any)
	return v, obj, true
}

// NonBlank returns the trimmed non-blank lines of data (lines up to
// 16 MiB long).
func NonBlank(data []byte) []string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	var out []string
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out
}
