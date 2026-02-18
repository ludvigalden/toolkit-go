// Package wiring holds the per-invocation plumbing behind cli: the
// environment prefix, session IDs, the slog level a verbosity implies,
// and the JSON-lines debug log sink.
package wiring

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/output"
	"github.com/ludvigalden/toolkit-go/pkg/severity"
)

// EnvPrefix returns override, or name upper-cased with dashes as
// underscores, always ending in exactly one "_".
func EnvPrefix(name, override string) string {
	p := override
	if p == "" {
		p = strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	}
	return strings.TrimSuffix(p, "_") + "_"
}

// SessionID returns 8 random hex characters.
func SessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b) //nolint:errcheck // crypto/rand.Read never fails on supported platforms
	return hex.EncodeToString(b)
}

// LevelFor maps the stdout verbosity onto the slog threshold library
// logging should use.
func LevelFor(v output.Verbosity) slog.Level {
	switch {
	case v >= output.VerbosityTrace:
		return severity.LevelTrace
	case v >= output.VerbosityDebug:
		return slog.LevelDebug
	case v >= output.VerbosityInfo:
		return slog.LevelInfo
	}
	return slog.LevelWarn
}

// DebugLog opens path for appending and returns a debug-level JSON
// handler over it; the caller closes the file.
func DebugLog(path string) (*os.File, slog.Handler, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("debug log: %w", err)
	}
	return f, slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}), nil
}
