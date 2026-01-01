// Package severity maps between slog levels, OpenTelemetry severity
// numbers (1-24) and CLI verbosity, so log filters, -v/-q flags and
// exported records agree on one scale.
package severity

import (
	"log/slog"
	"strconv"
)

// Number is an OpenTelemetry SeverityNumber (1-24).
type Number int

// The named severities. slog levels sit exactly on them: Number =
// slog.Level + 9, the OpenTelemetry slog bridge convention.
const (
	Trace Number = 1
	Debug Number = 5
	Info  Number = 9
	Warn  Number = 13
	Error Number = 17
	Fatal Number = 21
)

// LevelTrace is the slog level for trace output, below slog.LevelDebug.
const LevelTrace = slog.Level(-8)

// FromLevel maps a slog level onto the scale, clamped to [Trace, 24].
func FromLevel(l slog.Level) Number {
	return clamp(Number(int(l) + 9))
}

// Level maps a severity back to a slog level; the inverse of FromLevel
// within the scale.
func (n Number) Level() slog.Level { return slog.Level(int(n) - 9) }

// String names the severity range n falls in ("info", "warn2");
// out-of-range values render as their number.
func (n Number) String() string {
	if n < Trace || n > 24 {
		return strconv.Itoa(int(n))
	}
	names := [...]string{"trace", "debug", "info", "warn", "error", "fatal"}
	base := (int(n) - 1) / 4
	if off := (int(n) - 1) % 4; off > 0 {
		return names[base] + strconv.Itoa(off+1)
	}
	return names[base]
}

// Parse resolves a level name (trace, debug, info, warn, error, fatal)
// to its severity.
func Parse(name string) (Number, bool) {
	switch name {
	case "trace":
		return Trace, true
	case "debug":
		return Debug, true
	case "info":
		return Info, true
	case "warn", "warning":
		return Warn, true
	case "error":
		return Error, true
	case "fatal":
		return Fatal, true
	}
	return 0, false
}

// FromFlagCount maps the number of repeated -q/-v flags to a minimum
// severity: negative (quiet flags) is Error, 0 is Warn, 1 Info, 2
// Debug, 3+ Trace. It counts flag occurrences, a different scale from
// output.Verbosity (the 0-10 named verbosity); do not feed one into
// the other.
func FromFlagCount(v int) Number {
	switch {
	case v < 0:
		return Error
	case v == 0:
		return Warn
	case v == 1:
		return Info
	case v == 2:
		return Debug
	}
	return Trace
}

// LevelForFlagCount is FromFlagCount as a slog threshold.
func LevelForFlagCount(v int) slog.Level { return FromFlagCount(v).Level() }

func clamp(n Number) Number {
	if n < Trace {
		return Trace
	}
	if n > 24 {
		return 24
	}
	return n
}
