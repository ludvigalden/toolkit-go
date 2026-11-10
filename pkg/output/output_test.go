package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/diag"
	"github.com/ludvigalden/toolkit-go/pkg/errkind"
)

var fixed = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newTest(t *testing.T, opts ...Option) (*Output, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	o, err := New(&stdout, &stderr, append([]Option{WithClock(func() time.Time { return fixed })}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return o, &stdout, &stderr
}

func TestTextVerbosityGatesDiagnosticsNotResults(t *testing.T) {
	o, stdout, stderr := newTest(t)
	o.Info("hidden at warn")
	o.Debug("hidden")
	o.Warn("shown %d", 1)
	o.Result("payload")
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "payload\n" {
		t.Errorf("stdout = %q", got)
	}
	if got := stderr.String(); got != "03:04:05 WARN shown 1\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestInfoShownAtInfo(t *testing.T) {
	o, _, stderr := newTest(t, WithVerbosity(VerbosityInfo))
	o.Info("hello")
	o.Debug("still hidden")
	if got := stderr.String(); got != "03:04:05 INFO hello\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestJSONEnvelope(t *testing.T) {
	o, stdout, stderr := newTest(t, WithMode(ModeJSON))
	o.Result([]string{"a", "b"})
	o.Warn("careful")
	if stdout.Len() != 0 {
		t.Fatalf("json wrote before Close: %q", stdout)
	}
	_ = o.Close()
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout)
	}
	if doc["status"] != "ok" || doc["result_records"] != float64(2) {
		t.Errorf("doc = %v", doc)
	}
	if !strings.Contains(stderr.String(), `"kind":"warn"`) {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestFailCarriesKindAndCode(t *testing.T) {
	o, stdout, stderr := newTest(t, WithMode(ModeNDJSON))
	o.Result(map[string]int{"n": 1})
	o.Fail(errkind.New(errkind.NotFound, "no such note"))
	_ = o.Close()
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout lines = %q", lines)
	}
	var last map[string]any
	_ = json.Unmarshal([]byte(lines[1]), &last)
	if last["kind"] != "completed" || last["status"] != "failed" || last["code"] != "not-found" {
		t.Errorf("completed = %v", last)
	}
	if !strings.Contains(stderr.String(), "no such note") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestQuietKeepsOnlyFailures(t *testing.T) {
	o, stdout, stderr := newTest(t, WithMode(ModeQuiet), WithVerbosity(VerbosityMax))
	o.Warn("warn")
	o.Result("data")
	o.Fail(errors.New("boom"))
	_ = o.Close()
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q", stdout)
	}
	if got := stderr.String(); got != "03:04:05 FAIL boom\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestFailSurvivesQuietVerbosity(t *testing.T) {
	o, _, stderr := newTest(t, WithVerbosity(VerbosityQuiet))
	o.Error("dropped")
	o.Fail(errors.New("kept"))
	if got := stderr.String(); got != "03:04:05 FAIL kept\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestFailRendersDiagnostic(t *testing.T) {
	o, _, stderr := newTest(t)
	o.Fail(diag.Diagnostic{Code: "E1", Severity: diag.SeverityError, Pos: diag.Position{File: "a.yaml", Line: 3}, Message: "bad"})
	if got := stderr.String(); !strings.HasPrefix(got, "a.yaml:3: error[E1]: bad") {
		t.Errorf("stderr = %q", got)
	}
}

type card struct{ Name string }

func (c card) FormatText(a Args) string { return "\x1b[1m" + c.Name + "\x1b[0m" }

func TestTextFormattableStripsANSIWithoutColor(t *testing.T) {
	o, stdout, _ := newTest(t)
	o.Result(card{"x"})
	if got := stdout.String(); got != "x\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestYamlishResult(t *testing.T) {
	o, stdout, _ := newTest(t, WithMode(ModeYamlish))
	o.Result(map[string]any{"a": 1})
	if got := stdout.String(); got != "{a: 1}\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestUnknownModeIsError(t *testing.T) {
	if _, err := New(io.Discard, io.Discard, WithMode("custom")); err == nil {
		t.Fatal("a mode without a registered renderer should fail")
	}
}

type recorder struct{ logs []Record }

func (r *recorder) Log(rec Record)               { r.logs = append(r.logs, rec) }
func (r *recorder) Result(any)                   {}
func (r *recorder) Event(string, map[string]any) {}
func (r *recorder) Close(Outcome) error          { return nil }

func TestWithRendererAndCloseIdempotent(t *testing.T) {
	rec := &recorder{}
	o, err := New(io.Discard, io.Discard, WithMode("custom"), WithRenderer("custom", func(io.Writer, io.Writer, Args) (Renderer, error) { return rec, nil }))
	if err != nil {
		t.Fatal(err)
	}
	o.Warn("w")
	_ = o.Close()
	_ = o.Close()
	o.Warn("after close")
	if len(rec.logs) != 1 {
		t.Errorf("logs = %v", rec.logs)
	}
}

func TestLogHandlerSeesEverything(t *testing.T) {
	var log bytes.Buffer
	o, _, stderr := newTest(t, WithVerbosity(VerbositySilent), WithLogHandler(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	o.Debug("deep")
	o.Fail(errkind.New(errkind.Timeout, "slow"))
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q", stderr)
	}
	if !strings.Contains(log.String(), `"msg":"deep"`) || !strings.Contains(log.String(), `"code":"timeout"`) {
		t.Errorf("log = %s", log.String())
	}
}

func TestSlogHandler(t *testing.T) {
	o, _, stderr := newTest(t, WithVerbosity(VerbosityInfo))
	l := slog.New(NewSlogHandler(o, slog.LevelInfo)).WithGroup("g")
	l.Info("msg", "k", "v")
	l.Debug("dropped")
	if got := stderr.String(); got != "03:04:05 INFO msg {g.k: v}\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestParseVerbosity(t *testing.T) {
	for in, want := range map[string]Verbosity{"warn": VerbosityWarn, "DEBUG": VerbosityDebug, "0": VerbositySilent, " 10 ": VerbosityMax} {
		got, err := ParseVerbosity(in)
		if err != nil || got != want {
			t.Errorf("ParseVerbosity(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"11", "-1", "loud", ""} {
		if _, err := ParseVerbosity(bad); err == nil {
			t.Errorf("ParseVerbosity(%q) accepted", bad)
		}
	}
}

func TestColorModeEnabled(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	cases := []struct {
		m    ColorMode
		env  map[string]string
		want bool
	}{
		{ColorAlways, nil, true},
		{ColorAlways, map[string]string{"NO_COLOR": "1"}, false},
		{ColorAuto, map[string]string{"CLICOLOR_FORCE": "1"}, true},
		{ColorAuto, nil, false},
		{ColorNever, map[string]string{"CLICOLOR_FORCE": "1"}, false},
	}
	for _, c := range cases {
		if got := c.m.Enabled(io.Discard, env(c.env)); got != c.want {
			t.Errorf("%s %v = %v", c.m, c.env, got)
		}
	}
}

func TestHelpers(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1 KiB", 1536: "1.5 KiB", 10 << 20: "10 MiB"} {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", n, got, want)
		}
	}
	if Pluralize(1, "file", "files") != "file" || Pluralize(2, "file", "files") != "files" {
		t.Error("Pluralize")
	}
	if got := StripANSI("\x1b[38;5;42mok\x1b[0m"); got != "ok" {
		t.Errorf("StripANSI = %q", got)
	}
	if got := ErrorCode(fmt.Errorf("wrap: %w", &errkind.Error{Kind: errkind.Conflict, Code: "E42"})); got != "E42" {
		t.Errorf("ErrorCode = %q", got)
	}
}
