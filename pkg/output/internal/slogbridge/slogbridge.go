// Package slogbridge is the slog.Handler behind output.NewSlogHandler:
// it flattens a record's attributes into one budgeted yamlish suffix
// and hands the message to a sink by level.
package slogbridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/yamlish"
)

// Sink receives one rendered log line at its slog level.
type Sink func(level slog.Level, msg string)

// New returns a handler admitting records at or above level.
func New(sink Sink, level slog.Leveler) slog.Handler {
	if level == nil {
		level = slog.LevelInfo
	}
	return &handler{sink: sink, level: level}
}

type handler struct {
	sink   Sink
	level  slog.Leveler
	groups []string
	attrs  []slog.Attr
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	h.sink(r.Level, h.message(r))
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := *h
	n.attrs = append(append([]slog.Attr(nil), h.attrs...), h.qualify(attrs)...)
	return &n
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := *h
	n.groups = append(append([]string(nil), h.groups...), name)
	return &n
}

func (h *handler) qualify(attrs []slog.Attr) []slog.Attr {
	if len(h.groups) == 0 {
		return attrs
	}
	prefix := strings.Join(h.groups, ".") + "."
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = slog.Attr{Key: prefix + a.Key, Value: a.Value}
	}
	return out
}

func (h *handler) message(r slog.Record) string {
	m := map[string]any{}
	for _, a := range h.attrs {
		collect(m, "", a)
	}
	prefix := strings.Join(h.groups, ".")
	r.Attrs(func(a slog.Attr) bool {
		collect(m, prefix, a)
		return true
	})
	if len(m) == 0 {
		return r.Message
	}
	return r.Message + " " + yamlish.Format(m, yamlish.Options{MaxLineLen: 360, MaxTotalLen: 2000})
}

func collect(m map[string]any, prefix string, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindGroup:
		for _, g := range v.Group() {
			collect(m, key, g)
		}
	case slog.KindDuration:
		m[key] = v.Duration().String()
	case slog.KindTime:
		m[key] = v.Time().Format("2006-01-02 15:04:05")
	default:
		if err, ok := v.Any().(error); ok {
			m[key] = errorValue(err)
			return
		}
		if s, ok := v.Any().(fmt.Stringer); ok && v.Kind() == slog.KindAny {
			m[key] = s.String()
			return
		}
		m[key] = v.Any()
	}
}

// errorValue keeps an error's kind and code visible in log attributes
// and expands joined errors into a list.
func errorValue(err error) any {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var items []any
		for _, e := range j.Unwrap() {
			items = append(items, errorValue(e))
		}
		return items
	}
	var e *errkind.Error
	if errors.As(err, &e) {
		m := map[string]any{"message": err.Error(), "kind": e.Kind.Name()}
		if e.Code != "" {
			m["code"] = e.Code
		}
		return m
	}
	return err.Error()
}
