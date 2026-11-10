package output

import (
	"bytes"
	"io"
	"testing"
)

// page stands for more records than it carries.
type page struct{ Items []string }

func (p page) ResultCount() int { return 7 }

func TestCountableEnvelope(t *testing.T) {
	cases := map[Mode]string{
		ModeJSON: `{"result_records":8,"status":"ok","value":[{"Items":["a"]},"b"],"version":1}` + "\n",
		ModeNDJSON: `{"kind":"result","value":{"Items":["a"]},"version":1}` + "\n" +
			`{"kind":"result","value":"b","version":1}` + "\n" +
			`{"kind":"completed","result_records":2,"status":"ok","version":1}` + "\n",
	}
	for mode, want := range cases {
		var stdout bytes.Buffer
		o, err := New(&stdout, io.Discard, WithMode(mode))
		if err != nil {
			t.Fatal(err)
		}
		o.Result(page{Items: []string{"a"}})
		o.Result("b")
		_ = o.Close()
		if got := stdout.String(); got != want {
			t.Errorf("%s:\n got %q\nwant %q", mode, got, want)
		}
	}
}
