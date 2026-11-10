package output

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// Mode selects how results and diagnostics are rendered.
type Mode string

// The built-in modes. Other modes need a renderer registered with
// WithRenderer.
const (
	// ModeText writes results as readable text, diagnostics as lines.
	ModeText Mode = "text"
	// ModeJSON writes exactly one JSON document to stdout at Close.
	ModeJSON Mode = "json"
	// ModeNDJSON streams one result record per line, then a terminal
	// completed record carrying the status.
	ModeNDJSON Mode = "ndjson"
	// ModeYamlish writes results as budgeted YAML-like text.
	ModeYamlish Mode = "yamlish"
	// ModeQuiet writes no results; only failures reach stderr.
	ModeQuiet Mode = "quiet"
)

var knownModes = []Mode{ModeText, ModeJSON, ModeNDJSON, ModeYamlish, ModeQuiet}

// ParseMode parses a built-in mode name.
func ParseMode(s string) (Mode, error) {
	m := Mode(strings.TrimSpace(s))
	if slices.Contains(knownModes, m) {
		return m, nil
	}
	return "", fmt.Errorf("invalid output mode %q: must be one of text, json, ndjson, yamlish, quiet", s)
}

// Args tells payload formatters what they render for.
type Args struct {
	Mode      Mode
	Verbosity Verbosity
	Color     bool
}

// TextFormattable payloads render their own text in text-like modes.
// Output strips ANSI from the result when color is off.
type TextFormattable interface {
	FormatText(args Args) string
}

// DataFormattable payloads supply the value structured modes encode in
// place of the payload itself.
type DataFormattable interface {
	FormatData(args Args) any
}

// Kind classifies a diagnostic Record.
type Kind string

// The diagnostic kinds, least severe first.
const (
	KindDebug    Kind = "debug"
	KindInfo     Kind = "info"
	KindSuccess  Kind = "success"
	KindProgress Kind = "progress"
	KindWarn     Kind = "warn"
	KindError    Kind = "error"
)

// Record is one diagnostic. Err is set for failures reported through
// Output.Fail; Code is then its errkind or diag code.
type Record struct {
	Time    time.Time
	Kind    Kind
	Message string
	Code    string
	Err     error
}

// Outcome is how a run ended, handed to Renderer.Close.
type Outcome struct {
	// Err is the run's failure, nil for a successful run.
	Err error
	// Fields are extra top-level keys for structured final records.
	// Engine keys (version, kind, status, result_records, value,
	// message, code) are never overwritten.
	Fields map[string]any
}

// Renderer implements one Mode. Output serializes calls, so renderers
// need no locking of their own.
type Renderer interface {
	Log(r Record)
	Result(data any)
	Event(kind string, fields map[string]any)
	Close(o Outcome) error
}

// Factory builds a Renderer for one Output.
type Factory func(stdout, stderr io.Writer, args Args) (Renderer, error)

// Middleware wraps the Factory of whatever mode New selects, built-in
// or registered.
type Middleware func(next Factory) Factory
