package severity

import (
	"log/slog"
	"testing"
)

func TestLevelMapping(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  Number
	}{
		{LevelTrace, Trace},
		{slog.LevelDebug, Debug},
		{slog.LevelInfo, Info},
		{slog.LevelWarn, Warn},
		{slog.LevelError, Error},
		{slog.LevelError + 4, Fatal},
		{slog.Level(-100), Trace},
		{slog.Level(100), 24},
		{slog.LevelInfo + 1, 10},
	}
	for _, tt := range tests {
		got := FromLevel(tt.level)
		if got != tt.want {
			t.Errorf("FromLevel(%v) = %d, want %d", tt.level, got, tt.want)
		}
		if tt.level >= LevelTrace && tt.level <= 15 && got.Level() != tt.level {
			t.Errorf("round trip %v -> %d -> %v", tt.level, got, got.Level())
		}
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		n    Number
		want string
	}{
		{0, "0"},
		{Trace, "trace"},
		{2, "trace2"},
		{Debug, "debug"},
		{Info, "info"},
		{12, "info4"},
		{Warn, "warn"},
		{Error, "error"},
		{Fatal, "fatal"},
		{24, "fatal4"},
		{25, "25"},
		{-3, "-3"},
	}
	for _, tt := range tests {
		if got := tt.n.String(); got != tt.want {
			t.Errorf("Number(%d).String() = %q, want %q", int(tt.n), got, tt.want)
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		want Number
		ok   bool
	}{
		{"trace", Trace, true},
		{"debug", Debug, true},
		{"info", Info, true},
		{"warn", Warn, true},
		{"warning", Warn, true},
		{"error", Error, true},
		{"fatal", Fatal, true},
		{"INFO", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := Parse(tt.name)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Parse(%q) = %d, %v; want %d, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFlagCount(t *testing.T) {
	tests := []struct {
		v     int
		want  Number
		level slog.Level
	}{
		{-2, Error, slog.LevelError},
		{-1, Error, slog.LevelError},
		{0, Warn, slog.LevelWarn},
		{1, Info, slog.LevelInfo},
		{2, Debug, slog.LevelDebug},
		{3, Trace, LevelTrace},
		{9, Trace, LevelTrace},
	}
	for _, tt := range tests {
		if got := FromFlagCount(tt.v); got != tt.want {
			t.Errorf("FromFlagCount(%d) = %d, want %d", tt.v, got, tt.want)
		}
		if got := LevelForFlagCount(tt.v); got != tt.level {
			t.Errorf("LevelForFlagCount(%d) = %v, want %v", tt.v, got, tt.level)
		}
	}
}
