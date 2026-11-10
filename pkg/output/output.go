package output

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/diag"
	"github.com/ludvigalden/toolkit-go/pkg/errkind"
)

// Output routes one command's results and diagnostics. It is safe for
// concurrent use. Results always reach the renderer; diagnostics only
// when Verbosity admits their kind.
type Output struct {
	mu        sync.Mutex
	r         Renderer
	args      Args
	tees      []slog.Handler
	now       func() time.Time
	failed    error
	closed    bool
	closeErrs error
}

// Option configures New.
type Option func(*config)

type config struct {
	mode      Mode
	verbosity Verbosity
	color     bool
	factories map[Mode]Factory
	mw        []Middleware
	tees      []slog.Handler
	now       func() time.Time
}

// WithMode selects the rendering mode; the default is ModeText.
func WithMode(m Mode) Option { return func(c *config) { c.mode = m } }

// WithVerbosity sets the diagnostic threshold; the default is
// VerbosityWarn.
func WithVerbosity(v Verbosity) Option { return func(c *config) { c.verbosity = v } }

// WithColor enables ANSI color in text-like modes.
func WithColor(on bool) Option { return func(c *config) { c.color = on } }

// WithRenderer supplies the Factory for mode, replacing a built-in one.
// It is how modes outside the built-in set are attached.
func WithRenderer(m Mode, f Factory) Option {
	return func(c *config) {
		if c.factories == nil {
			c.factories = map[Mode]Factory{}
		}
		c.factories[m] = f
	}
}

// WithMiddleware wraps the selected mode's Factory. Repeatable; the
// first registered is outermost.
func WithMiddleware(m ...Middleware) Option {
	return func(c *config) { c.mw = append(c.mw, m...) }
}

// WithLogHandler mirrors every diagnostic to h, whatever the
// verbosity, so a log file can keep detail the terminal hides.
// Repeatable; each handler receives every record.
func WithLogHandler(h slog.Handler) Option {
	return func(c *config) { c.tees = append(c.tees, h) }
}

// WithClock replaces time.Now for diagnostic timestamps.
func WithClock(now func() time.Time) Option { return func(c *config) { c.now = now } }

// New builds an Output writing results to stdout and diagnostics to
// stderr. A mode without a renderer is an error, never a silent
// fallback.
func New(stdout, stderr io.Writer, opts ...Option) (*Output, error) {
	c := config{mode: ModeText, verbosity: VerbosityWarn, now: time.Now}
	for _, o := range opts {
		o(&c)
	}
	f, ok := c.factories[c.mode]
	if !ok {
		f, ok = builtin(c.mode)
	}
	if !ok {
		return nil, fmt.Errorf("output: no renderer for mode %q", c.mode)
	}
	for i := len(c.mw) - 1; i >= 0; i-- {
		f = c.mw[i](f)
	}
	args := Args{Mode: c.mode, Verbosity: c.verbosity, Color: c.color}
	r, err := f(stdout, stderr, args)
	if err != nil {
		return nil, fmt.Errorf("output: %s renderer: %w", c.mode, err)
	}
	return &Output{r: r, args: args, tees: c.tees, now: c.now}, nil
}

func builtin(m Mode) (Factory, bool) {
	switch m {
	case ModeText:
		return TextRenderer, true
	case ModeYamlish:
		return newYamlish, true
	case ModeJSON:
		return newJSON, true
	case ModeNDJSON:
		return newNDJSON, true
	case ModeQuiet:
		return newQuiet, true
	}
	return nil, false
}

// Args reports what the renderer was built for.
func (o *Output) Args() Args { return o.args }

// Debug reports detail shown at VerbosityDebug.
func (o *Output) Debug(msg string, args ...any) { o.log(KindDebug, nil, msg, args) }

// Info reports progress narration shown at VerbosityInfo.
func (o *Output) Info(msg string, args ...any) { o.log(KindInfo, nil, msg, args) }

// Success reports a completed step, shown at VerbosityInfo.
func (o *Output) Success(msg string, args ...any) { o.log(KindSuccess, nil, msg, args) }

// Progress reports advancement of a long step, shown at VerbosityInfo.
func (o *Output) Progress(msg string, args ...any) { o.log(KindProgress, nil, msg, args) }

// Notice is information shown by default, not a warning.
func (o *Output) Notice(msg string, args ...any) { o.log(KindNotice, nil, msg, args) }

// Warn reports something the user should act on, shown by default.
func (o *Output) Warn(msg string, args ...any) { o.log(KindWarn, nil, msg, args) }

// Error reports a failure that does not end the run.
func (o *Output) Error(msg string, args ...any) { o.log(KindError, nil, msg, args) }

// Fail records err as the run's outcome and reports it. Structured
// modes carry it in their final record; Close then reports the run as
// failed. errkind and diag errors keep their kind and code.
func (o *Output) Fail(err error) {
	if err == nil {
		return
	}
	o.mu.Lock()
	if o.failed == nil {
		o.failed = err
	}
	o.mu.Unlock()
	o.log(KindError, err, "%s", []any{err.Error()})
}

// Result emits one data payload. A slice is a list of records.
func (o *Output) Result(data any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.closed {
		o.r.Result(data)
	}
}

// Event writes a structured side record: ndjson puts it on stdout as
// {kind, version, ...fields}, json on stderr so stdout stays one
// document; text modes drop it.
func (o *Output) Event(kind string, fields map[string]any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.closed {
		o.r.Event(kind, fields)
	}
}

// Close finishes the stdout document. It is idempotent.
func (o *Output) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.closed {
		o.closed = true
		o.closeErrs = o.r.Close(Outcome{Err: o.failed})
	}
	return o.closeErrs
}

func (o *Output) log(k Kind, err error, msg string, args []any) {
	rec := Record{Time: o.now(), Kind: k, Message: fmt.Sprintf(msg, args...), Err: err}
	if err != nil {
		rec.Code = ErrorCode(err)
	}
	for _, h := range o.tees {
		mirror(h, rec)
	}
	if !o.args.Verbosity.admits(k, err != nil) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.closed {
		o.r.Log(rec)
	}
}

func mirror(h slog.Handler, rec Record) {
	lvl := kindLevel(rec.Kind)
	if !h.Enabled(context.Background(), lvl) {
		return
	}
	r := slog.NewRecord(rec.Time, lvl, rec.Message, 0)
	if rec.Code != "" {
		r.AddAttrs(slog.String("code", rec.Code))
	}
	_ = h.Handle(context.Background(), r) //nolint:errcheck // a failing log sink must not fail the command
}

// ErrorCode returns the most specific code err carries: a diag code,
// then an errkind code, then the errkind name; "" when unclassified.
func ErrorCode(err error) string {
	var d diag.Diagnostic
	if errors.As(err, &d) && d.Code != "" {
		return d.Code
	}
	var e *errkind.Error
	if errors.As(err, &e) && e.Code != "" {
		return e.Code
	}
	if k, ok := errkind.KindOf(err); ok {
		return k.Name()
	}
	return ""
}

func kindLevel(k Kind) slog.Level {
	switch k {
	case KindDebug:
		return slog.LevelDebug
	case KindWarn:
		return slog.LevelWarn
	case KindError:
		return slog.LevelError
	}
	return slog.LevelInfo
}
