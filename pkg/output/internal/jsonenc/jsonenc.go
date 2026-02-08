// Package jsonenc writes the json and ndjson output documents: one
// envelope at Close (json) or one record per line then a completed
// record (ndjson). Diagnostics are NDJSON lines on stderr in both.
package jsonenc

import (
	"encoding/json"
	"io"
	"reflect"
	"time"
)

// Writer accumulates or streams result records. It is not safe for
// concurrent use; output serializes calls.
type Writer struct {
	stdout, stderr *json.Encoder
	stream         bool
	values         []any
	count          int
}

// New returns a Writer; stream selects ndjson.
func New(stdout, stderr io.Writer, stream bool) *Writer {
	w := &Writer{stdout: encoder(stdout), stderr: encoder(stderr), stream: stream}
	if !stream {
		w.values = []any{}
	}
	return w
}

func encoder(w io.Writer) *json.Encoder {
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	return e
}

// Diagnostic writes one diagnostic line to stderr.
func (w *Writer) Diagnostic(kind string, t time.Time, msg, code string) {
	line := map[string]any{
		"kind":      kind,
		"timestamp": t.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"version":   1,
		"message":   msg,
	}
	if code != "" {
		line["code"] = code
	}
	_ = w.stderr.Encode(line) //nolint:errcheck // stderr write failures are not actionable
}

// Result adds data; a slice (other than []byte) contributes one record
// per element.
func (w *Writer) Result(data any) {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		for i := range v.Len() {
			w.record(v.Index(i).Interface())
		}
		return
	}
	w.record(data)
}

func (w *Writer) record(v any) {
	w.count++
	if w.stream {
		_ = w.stdout.Encode(map[string]any{"kind": "result", "version": 1, "value": v}) //nolint:errcheck // a closed stdout ends the stream
		return
	}
	w.values = append(w.values, v)
}

// Failure describes a failed run for the closing record.
type Failure struct {
	Message string
	Code    string
}

// Close writes the envelope (json) or the completed record (ndjson):
// status, result_records, and the failure's message and code.
func (w *Writer) Close(failed *Failure) error {
	doc := map[string]any{"version": 1, "status": "ok", "result_records": w.count}
	if w.stream {
		doc["kind"] = "completed"
	} else {
		doc["value"] = w.values
	}
	if failed != nil {
		doc["status"] = "failed"
		doc["message"] = failed.Message
		if failed.Code != "" {
			doc["code"] = failed.Code
		}
	}
	return w.stdout.Encode(doc)
}
