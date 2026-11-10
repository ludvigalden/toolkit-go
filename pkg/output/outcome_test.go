package output

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// withFields is a Renderer that adds final-record fields at Close.
type withFields struct {
	Renderer
	fields map[string]any
}

func (w withFields) Close(o Outcome) error {
	o.Fields = w.fields
	return w.Renderer.Close(o)
}

func fieldsMiddleware(fields map[string]any) Option {
	return WithMiddleware(func(next Factory) Factory {
		return func(stdout, stderr io.Writer, args Args) (Renderer, error) {
			r, err := next(stdout, stderr, args)
			if err != nil {
				return nil, err
			}
			return withFields{r, fields}, nil
		}
	})
}

func TestOutcomeFieldsGoldens(t *testing.T) {
	fields := map[string]any{"run_state": "complete", "status": "hijacked", "value": "hijacked", "code": "X"}
	cases := []struct {
		mode Mode
		fail bool
		want string
	}{
		{ModeJSON, false, `{"result_records":1,"run_state":"complete","status":"ok","value":["a"],"version":1}` + "\n"},
		{ModeJSON, true, `{"message":"boom","result_records":1,"run_state":"complete","status":"failed","value":["a"],"version":1}` + "\n"},
		{ModeNDJSON, false, `{"kind":"result","value":"a","version":1}` + "\n" +
			`{"kind":"completed","result_records":1,"run_state":"complete","status":"ok","version":1}` + "\n"},
		{ModeText, false, "a\n"},
		{ModeYamlish, false, "a\n"},
		{ModeQuiet, false, ""},
	}
	for _, c := range cases {
		var stdout bytes.Buffer
		o, err := New(&stdout, io.Discard, WithMode(c.mode), fieldsMiddleware(fields))
		if err != nil {
			t.Fatal(err)
		}
		o.Result("a")
		if c.fail {
			o.Fail(errors.New("boom"))
		}
		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); got != c.want {
			t.Errorf("%s fail=%v:\n got %q\nwant %q", c.mode, c.fail, got, c.want)
		}
	}
}
