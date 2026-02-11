package output

import (
	"log/slog"

	"github.com/ludvigalden/toolkit-go/pkg/output/internal/slogbridge"
)

// NewSlogHandler routes slog records into out as diagnostics, so
// library logging obeys the same verbosity and mode as the command.
// Attributes render inline after the message.
func NewSlogHandler(out *Output, level slog.Leveler) slog.Handler {
	return slogbridge.New(func(l slog.Level, msg string) {
		switch {
		case l >= slog.LevelError:
			out.Error("%s", msg)
		case l >= slog.LevelWarn:
			out.Warn("%s", msg)
		case l >= slog.LevelInfo:
			out.Info("%s", msg)
		default:
			out.Debug("%s", msg)
		}
	}, level)
}
