package diag

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPositionString(t *testing.T) {
	tests := []struct {
		pos  Position
		want string
	}{
		{Position{}, ""},
		{Position{Line: 3}, ""},
		{Position{File: "a.yaml"}, "a.yaml"},
		{Position{File: "a.yaml", Line: 3}, "a.yaml:3"},
		{Position{File: "a.yaml", Line: 3, Col: 7}, "a.yaml:3:7"},
		{Position{File: "a.yaml", Col: 7}, "a.yaml"},
	}
	for _, tt := range tests {
		if got := tt.pos.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.pos, got, tt.want)
		}
	}
}

func TestFormatPretty(t *testing.T) {
	tests := []struct {
		name string
		ds   []Diagnostic
		want string
	}{
		{"empty", nil, ""},
		{
			"full",
			[]Diagnostic{{
				Code: "cfg.bad", Severity: SeverityError,
				Pos:     Position{File: "tool.yaml", Line: 4, Col: 2, Path: "server.port"},
				Message: "port out of range",
				Fix:     &Fix{Command: "tool config fix", Description: "resets the port"},
			}},
			"tool.yaml:4:2: error[cfg.bad]: port out of range (at server.port)\n  fix: tool config fix (resets the port)\n",
		},
		{
			"no position no code",
			[]Diagnostic{{Severity: SeverityWarning, Message: "deprecated"}},
			"warning: deprecated\n",
		},
		{
			"empty severity defaults to error",
			[]Diagnostic{{Message: "boom"}},
			"error: boom\n",
		},
		{
			"fix command only and description only",
			[]Diagnostic{
				{Severity: SeverityHint, Message: "a", Fix: &Fix{Command: "run x"}},
				{Severity: SeverityInfo, Message: "b", Fix: &Fix{Description: "do y"}},
				{Severity: SeverityInfo, Message: "c", Fix: &Fix{}},
			},
			"hint: a\n  fix: run x\ninfo: b\n  fix: do y\ninfo: c\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Format(&buf, tt.ds, Style{}); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestFormatColor(t *testing.T) {
	var buf bytes.Buffer
	ds := []Diagnostic{{Severity: SeverityError, Code: "x", Pos: Position{File: "f"}, Message: "m"}}
	if err := Format(&buf, ds, Style{Color: true}); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[1mf\x1b[0m: \x1b[1;31merror[x]\x1b[0m: m\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	for _, s := range []Severity{SeverityWarning, SeverityInfo, SeverityHint, "other"} {
		if severityColor(s) == "" {
			t.Errorf("no colour for %q", s)
		}
	}
}

func TestFormatJSON(t *testing.T) {
	tests := []struct {
		name string
		ds   []Diagnostic
		want string
	}{
		{"nil", nil, "[]\n"},
		{
			"zero severity",
			[]Diagnostic{{Message: "boom"}},
			`[{"severity":"error","message":"boom"}]` + "\n",
		},
		{
			"minimal",
			[]Diagnostic{{Severity: SeverityWarning, Message: "a<b"}},
			`[{"severity":"warning","message":"a<b"}]` + "\n",
		},
		{
			"full",
			[]Diagnostic{{
				Code: "c", Severity: SeverityError,
				Pos:     Position{File: "f", Line: 1, Col: 2, Path: "p"},
				Message: "m", Fix: &Fix{Command: "x"},
			}},
			`[{"code":"c","severity":"error","pos":{"file":"f","line":1,"col":2,"path":"p"},"message":"m","fix":{"command":"x"}}]` + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Format(&buf, tt.ds, Style{JSON: true, Color: true}); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("got %s want %s", got, tt.want)
			}
			var round []Diagnostic
			if err := json.Unmarshal(buf.Bytes(), &round); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFormatWriteError(t *testing.T) {
	ds := []Diagnostic{{Message: "m"}}
	for _, st := range []Style{{}, {JSON: true}} {
		if err := Format(failWriter{}, ds, st); err == nil || !strings.HasPrefix(err.Error(), "diag:") {
			t.Errorf("style %+v: err = %v", st, err)
		}
	}
}

func TestHasErrorsAndError(t *testing.T) {
	if HasErrors(nil) || HasErrors([]Diagnostic{{Severity: SeverityWarning}}) {
		t.Error("HasErrors true without errors")
	}
	if !HasErrors([]Diagnostic{{Severity: SeverityHint}, {Severity: SeverityError}}) {
		t.Error("HasErrors false with an error")
	}
	var err error = Diagnostic{Severity: SeverityError, Code: "c", Message: "m"}
	if err.Error() != "error[c]: m" {
		t.Errorf("Error() = %q", err.Error())
	}
}
