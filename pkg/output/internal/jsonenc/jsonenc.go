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

// Event writes {kind, version, ...fields}: on stdout when streaming,
// on stderr otherwise so the json document stays whole. kind and
// version win over same-named fields.
func (w *Writer) Event(kind string, fields map[string]any) {
	line := make(map[string]any, len(fields)+2)
	for k, v := range fields {
		line[k] = v
	}
	line["kind"], line["version"] = kind, 1
	enc := w.stderr
	if w.stream {
		enc = w.stdout
	}
	_ = enc.Encode(line) //nolint:errcheck // a closed stream ends the run's records
}

// Result adds data; a slice (other than []byte) contributes one record
// per element. A non-negative count replaces the envelope's tally for
// this payload; streaming always counts the lines written.
func (w *Writer) Result(data any, count int) {
	before := w.count
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		for i := range v.Len() {
			w.record(v.Index(i).Interface())
		}
	} else {
		w.record(data)
	}
	if count >= 0 && !w.stream {
		w.count = before + count
	}
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

// reserved reports the engine keys a caller's fields never replace.
func reserved(k string) bool {
	switch k {
	case "version", "kind", "status", "result_records", "value", "message", "code":
		return true
	}
	return false
}

// Close writes the envelope (json) or the completed record (ndjson):
// status, result_records, the failure's message and code, and fields
// except reserved keys.
func (w *Writer) Close(failed *Failure, fields map[string]any) error {
	doc := map[string]any{}
	for k, v := range fields {
		if !reserved(k) {
			doc[k] = v
		}
	}
	doc["version"], doc["status"], doc["result_records"] = 1, "ok", w.count
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
