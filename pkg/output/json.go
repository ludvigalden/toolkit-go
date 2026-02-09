package output

import (
	"io"

	"github.com/ludvigalden/toolkit-go/pkg/output/internal/jsonenc"
)

// jsonRenderer adapts jsonenc to Renderer for json and ndjson.
type jsonRenderer struct {
	w    *jsonenc.Writer
	args Args
}

func newJSON(stdout, stderr io.Writer, args Args) (Renderer, error) {
	return &jsonRenderer{w: jsonenc.New(stdout, stderr, false), args: args}, nil
}

func newNDJSON(stdout, stderr io.Writer, args Args) (Renderer, error) {
	return &jsonRenderer{w: jsonenc.New(stdout, stderr, true), args: args}, nil
}

func (j *jsonRenderer) Log(r Record) { j.w.Diagnostic(string(r.Kind), r.Time, r.Message, r.Code) }

func (j *jsonRenderer) Result(data any) { j.w.Result(resolveData(data, j.args)) }

func (j *jsonRenderer) Close(failed error) error {
	if failed == nil {
		return j.w.Close(nil)
	}
	return j.w.Close(&jsonenc.Failure{Message: failed.Error(), Code: ErrorCode(failed)})
}
