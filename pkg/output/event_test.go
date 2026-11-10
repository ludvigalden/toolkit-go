package output

import (
	"bytes"
	"testing"
)

func TestEventGoldens(t *testing.T) {
	cases := []struct {
		mode           Mode
		stdout, stderr string
	}{
		{
			ModeNDJSON,
			`{"kind":"run_state","state":"partial","version":1}` + "\n" +
				`{"kind":"completed","result_records":0,"status":"ok","version":1}` + "\n",
			"",
		},
		{
			ModeJSON,
			`{"result_records":0,"status":"ok","value":[],"version":1}` + "\n",
			`{"kind":"run_state","state":"partial","version":1}` + "\n",
		},
		{ModeText, "", ""},
		{ModeYamlish, "", ""},
		{ModeQuiet, "", ""},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		o, err := New(&stdout, &stderr, WithMode(c.mode))
		if err != nil {
			t.Fatal(err)
		}
		o.Event("run_state", map[string]any{"state": "partial", "kind": "ignored"})
		_ = o.Close()
		o.Event("late", nil)
		if stdout.String() != c.stdout || stderr.String() != c.stderr {
			t.Errorf("%s:\nstdout %q\nstderr %q", c.mode, stdout.String(), stderr.String())
		}
	}
}
