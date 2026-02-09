package output

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/diag"
	"github.com/ludvigalden/toolkit-go/pkg/output/internal/textfmt"
	"github.com/ludvigalden/toolkit-go/pkg/yamlish"
)

// TextRenderer is the ModeText Factory, exported so other renderers can
// fall back to or wrap plain text rendering.
func TextRenderer(stdout, stderr io.Writer, args Args) (Renderer, error) {
	return &textRenderer{stdout: stdout, stderr: stderr, args: args}, nil
}

func newYamlish(stdout, stderr io.Writer, args Args) (Renderer, error) {
	return &textRenderer{stdout: stdout, stderr: stderr, args: args, yamlish: true}, nil
}

func newQuiet(_, stderr io.Writer, args Args) (Renderer, error) {
	return &textRenderer{stdout: io.Discard, stderr: stderr, args: args, quiet: true}, nil
}

// textRenderer serves text, yamlish and quiet: they differ only in how
// a result is rendered and whether anything but failures is shown.
type textRenderer struct {
	stdout, stderr io.Writer
	args           Args
	yamlish        bool
	quiet          bool
}

func (t *textRenderer) Log(r Record) {
	if t.quiet && r.Err == nil {
		return
	}
	var d diag.Diagnostic
	if r.Err != nil && errors.As(r.Err, &d) {
		_ = diag.Format(t.stderr, []diag.Diagnostic{d}, diag.Style{Color: t.args.Color}) //nolint:errcheck // stderr write failures are not actionable
		return
	}
	line := textfmt.Line(r.Time, string(r.Kind), r.Err != nil, r.Code, r.Message, t.args.Color)
	_, _ = fmt.Fprintln(t.stderr, line)
}

func (t *textRenderer) Result(data any) {
	if t.quiet {
		return
	}
	var s string
	if t.yamlish {
		s = yamlish.Format(resolveData(data, t.args), yamlish.Options{UseColor: t.args.Color})
	} else {
		s = renderText(data, t.args)
	}
	if s = strings.TrimRight(s, "\n"); s != "" {
		_, _ = fmt.Fprintln(t.stdout, s)
	}
}

func (t *textRenderer) Close(error) error { return nil }

// renderText renders data for text mode: its own text when it has one,
// strings verbatim, everything else as unbudgeted yamlish.
func renderText(data any, args Args) string {
	if f, ok := data.(TextFormattable); ok {
		s := f.FormatText(args)
		if !args.Color {
			s = textfmt.StripANSI(s)
		}
		return s
	}
	if s, ok := data.(string); ok {
		return s
	}
	return yamlish.Format(resolveData(data, args), yamlish.Options{
		UseColor:          args.Color,
		MaxStrLen:         1 << 30,
		MaxTotalLen:       1 << 30,
		NeverInlineArrays: true,
	})
}

func resolveData(data any, args Args) any {
	if f, ok := data.(DataFormattable); ok {
		return f.FormatData(args)
	}
	return data
}
