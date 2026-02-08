package output

import (
	"fmt"
	"io"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/output/internal/textfmt"
)

// ColorMode is the user's color preference.
type ColorMode string

// The color preferences.
const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

// ParseColorMode parses auto, always or never; empty means auto.
func ParseColorMode(s string) (ColorMode, error) {
	switch m := ColorMode(s); m {
	case "":
		return ColorAuto, nil
	case ColorAuto, ColorAlways, ColorNever:
		return m, nil
	}
	return "", fmt.Errorf("invalid color mode %q: must be auto, always or never", s)
}

// Enabled resolves the preference for w. NO_COLOR wins over everything,
// then the explicit mode, then CLICOLOR_FORCE and CLICOLOR, then
// whether w is a terminal. A nil getenv means os.Getenv.
func (m ColorMode) Enabled(w io.Writer, getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	return textfmt.ColorEnabled(string(m), w, getenv)
}

// IsTerminal reports whether w is a terminal device.
func IsTerminal(w io.Writer) bool { return textfmt.IsTerminal(w) }

// StripANSI removes ANSI escape sequences from s.
func StripANSI(s string) string { return textfmt.StripANSI(s) }

// Bytes formats n in binary units: "1023 B", "1.5 KiB", "12 MiB".
func Bytes(n int64) string { return textfmt.Bytes(n) }

// Pluralize returns singular when n is 1, plural otherwise.
func Pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
