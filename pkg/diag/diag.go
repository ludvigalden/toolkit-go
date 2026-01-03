// Package diag carries positioned, coded diagnostics and renders them
// either for people (compiler-style lines on stderr) or for machines
// (JSON), so every tool reports findings in one recognisable shape.
package diag

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Severity ranks a diagnostic. Only SeverityError should fail a run.
type Severity string

// The diagnostic severities, most severe first.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
	SeverityHint    Severity = "hint"
)

// Position locates a diagnostic. Line and Col are 1-based, zero when
// unknown. Path is a structural location inside the file, such as a
// dotted config key, for findings a line number cannot pin.
type Position struct {
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
	Path string `json:"path,omitempty"`
}

// String renders "file:line:col", omitting unknown parts; it is empty
// when File is.
func (p Position) String() string {
	if p.File == "" {
		return ""
	}
	s := p.File
	if p.Line > 0 {
		s += ":" + strconv.Itoa(p.Line)
		if p.Col > 0 {
			s += ":" + strconv.Itoa(p.Col)
		}
	}
	return s
}

// Fix is a suggested remedy: a command to run and/or a description.
type Fix struct {
	Command     string `json:"command,omitempty"`
	Description string `json:"description,omitempty"`
}

// Diagnostic is one finding. Code is a stable machine-readable
// identifier; Fix is nil when no remedy is known.
type Diagnostic struct {
	Code     string   `json:"code,omitempty"`
	Severity Severity `json:"severity"`
	Pos      Position `json:"pos,omitzero"`
	Message  string   `json:"message"`
	Fix      *Fix     `json:"fix,omitempty"`
}

// String renders the diagnostic as one uncoloured pretty line, without
// the fix.
func (d Diagnostic) String() string {
	var b strings.Builder
	writeHead(&b, d, false)
	return b.String()
}

// Error lets a single diagnostic travel as an error value.
func (d Diagnostic) Error() string { return d.String() }

// HasErrors reports whether any diagnostic has SeverityError.
func HasErrors(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Style selects the rendering: JSON writes one JSON array; otherwise
// pretty lines, with ANSI colour when Color is set.
type Style struct {
	JSON  bool
	Color bool
}

// Format writes ds to w in the given style. A nil or empty ds writes
// "[]" in JSON mode and nothing in pretty mode.
func Format(w io.Writer, ds []Diagnostic, style Style) error {
	if style.JSON {
		if ds == nil {
			ds = []Diagnostic{}
		}
		// A zero Severity serializes as the default the pretty path
		// substitutes, so both surfaces agree.
		ds = append([]Diagnostic{}, ds...)
		for i := range ds {
			if ds[i].Severity == "" {
				ds[i].Severity = SeverityError
			}
		}
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(ds); err != nil {
			return fmt.Errorf("diag: encode: %w", err)
		}
		return nil
	}
	var b strings.Builder
	for _, d := range ds {
		writeHead(&b, d, style.Color)
		b.WriteByte('\n')
		if d.Fix != nil {
			writeFix(&b, *d.Fix)
		}
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("diag: write: %w", err)
	}
	return nil
}

// ANSI SGR sequences for the pretty renderer.
const (
	sgrReset = "\x1b[0m"
	sgrBold  = "\x1b[1m"
)

func severityColor(s Severity) string {
	switch s {
	case SeverityError:
		return "\x1b[1;31m"
	case SeverityWarning:
		return "\x1b[1;33m"
	case SeverityInfo:
		return "\x1b[1;34m"
	case SeverityHint:
		return "\x1b[1;36m"
	}
	return sgrBold
}

// writeHead renders "file:line:col: severity[code]: message (at path)".
func writeHead(b *strings.Builder, d Diagnostic, color bool) {
	if pos := d.Pos.String(); pos != "" {
		if color {
			b.WriteString(sgrBold + pos + sgrReset)
		} else {
			b.WriteString(pos)
		}
		b.WriteString(": ")
	}
	sev := string(d.Severity)
	if sev == "" {
		sev = string(SeverityError)
	}
	if d.Code != "" {
		sev += "[" + d.Code + "]"
	}
	if color {
		sev = severityColor(d.Severity) + sev + sgrReset
	}
	b.WriteString(sev)
	b.WriteString(": ")
	b.WriteString(d.Message)
	if d.Pos.Path != "" {
		b.WriteString(" (at " + d.Pos.Path + ")")
	}
}

// writeFix renders the indented remedy line under a diagnostic.
func writeFix(b *strings.Builder, f Fix) {
	switch {
	case f.Command != "" && f.Description != "":
		b.WriteString("  fix: " + f.Command + " (" + f.Description + ")\n")
	case f.Command != "":
		b.WriteString("  fix: " + f.Command + "\n")
	case f.Description != "":
		b.WriteString("  fix: " + f.Description + "\n")
	}
}
