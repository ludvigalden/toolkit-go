package procrun

import (
	"errors"

	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/linestream"
)

// LineReader reads streamed stdout line by line, without newlines.
type LineReader struct {
	ch <-chan string
}

// Next returns the next line, or ("", false) once output is exhausted. It
// returns at once for a Process started with Run.
func (r *LineReader) Next() (string, bool) {
	if r.ch == nil {
		return "", false
	}
	line, ok := <-r.ch
	return line, ok
}

// Lines returns the line channel, closed when output ends.
func (r *LineReader) Lines() <-chan string { return r.ch }

// RawReader reads streamed stdout as byte chunks.
type RawReader struct {
	ch <-chan []byte
}

// Next returns the next chunk, or (nil, false) once output is exhausted.
func (r *RawReader) Next() ([]byte, bool) {
	if r.ch == nil {
		return nil, false
	}
	chunk, ok := <-r.ch
	return chunk, ok
}

// BytesCh returns the chunk channel, closed when output ends.
func (r *RawReader) BytesCh() <-chan []byte { return r.ch }

// EventKind classifies a stream event.
type EventKind int

const (
	// KindJSON is a line that decoded as JSON (Value set; Data for objects).
	KindJSON EventKind = iota
	// KindNonJSON is a line that did not decode as JSON.
	KindNonJSON
	// KindLine is a verbatim line from StreamLines.
	KindLine
	// KindExit is the final event: ExitCode and Err carry the outcome.
	KindExit
)

// Event is one line, or the terminal exit, of a subprocess stream. Line
// is 1-based; Value and Data carry a decoded KindJSON line (Data only for
// objects); ExitCode and Err carry KindExit's outcome.
type Event struct {
	Kind     EventKind
	Line     int
	Value    any
	Data     map[string]any
	Raw      string
	ExitCode int
	Err      error
}

// StreamJSON feeds each stdout line of a Start-ed proc to fn, decoded as
// JSON when possible, then a final KindExit event. fn returning false
// kills the process. It returns Wait's error.
func StreamJSON(proc *Process, fn func(Event) bool) error {
	return stream(proc, fn, decodeLine)
}

// StreamLines is StreamJSON without decoding: every line is KindLine.
func StreamLines(proc *Process, fn func(Event) bool) error {
	return stream(proc, fn, func(n int, line string) Event {
		return Event{Kind: KindLine, Line: n, Raw: line}
	})
}

// ParseLines classifies newline-separated captured output (for example
// Result.Stdout) like StreamJSON, skipping blank lines; no KindExit.
func ParseLines(data []byte) []Event {
	var events []Event
	for _, line := range linestream.NonBlank(data) {
		events = append(events, decodeLine(len(events)+1, line))
	}
	return events
}

// stream drives one line loop and the terminal exit event.
func stream(proc *Process, fn func(Event) bool, mk func(int, string) Event) error {
	lr := proc.Stdout()
	for n := 1; ; n++ {
		line, ok := lr.Next()
		if !ok {
			break
		}
		if !fn(mk(n, line)) {
			_ = proc.Kill() //nolint:errcheck // Wait below reaps regardless
			break
		}
	}
	res, err := proc.Wait()
	code := -1
	var ee *ExitError
	switch {
	case err == nil:
		code = res.ExitCode
	case errors.As(err, &ee):
		code = ee.ExitCode
	}
	fn(Event{Kind: KindExit, ExitCode: code, Err: err})
	return err
}

// decodeLine classifies one line as KindJSON or KindNonJSON.
func decodeLine(n int, line string) Event {
	e := Event{Kind: KindNonJSON, Line: n, Raw: line}
	if v, obj, ok := linestream.Decode(line); ok {
		e.Kind, e.Value, e.Data = KindJSON, v, obj
	}
	return e
}
