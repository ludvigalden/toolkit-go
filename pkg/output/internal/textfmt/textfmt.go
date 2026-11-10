// Package textfmt holds the terminal text mechanics behind output:
// diagnostic line layout, the ANSI palette and stripping, color
// resolution from the environment, and byte-size formatting.
package textfmt

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/term"
)

// ANSI palette for diagnostic lines and tool text.
const (
	Reset   = "\033[0m"
	Dim     = "\033[38;5;240m"
	Info    = "\033[38;5;245m"
	Warn    = "\033[38;5;214m"
	Error   = "\033[38;5;196m"
	Success = "\033[38;5;42m"
	Key     = "\033[36m"

	bold     = "\033[1m"
	progress = "\033[38;5;51m"
)

// Line renders "HH:MM:SS LABEL [code] message" for a diagnostic of
// kind; failure marks an error given to Output.Fail.
func Line(t time.Time, kind string, failure bool, code, msg string, color bool) string {
	label, tint := style(kind, failure)
	if code != "" {
		msg = "[" + code + "] " + msg
	}
	ts := t.Format("15:04:05")
	if !color {
		return ts + " " + label + " " + msg
	}
	return Dim + ts + Reset + " " + tint + label + Reset + " " + msg
}

func style(kind string, failure bool) (label, tint string) {
	switch kind {
	case "debug":
		return "DBUG", Info
	case "success":
		return "OK", Success
	case "progress":
		return "PROG", progress
	case "warn":
		return "WARN", Warn
	case "error":
		if failure {
			return "FAIL", bold + Error
		}
		return "ERR", Error
	}
	return "INFO", Info
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// StripANSI removes ANSI escape sequences from s.
func StripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

// IsTerminal reports whether w is a terminal device.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// ColorEnabled resolves a color preference (auto, always, never) for
// w: NO_COLOR, then the explicit mode, then CLICOLOR_FORCE and
// CLICOLOR, then whether w is a terminal. getenv must be non-nil.
func ColorEnabled(mode string, w io.Writer, getenv func(string) string) bool {
	if getenv("NO_COLOR") != "" || mode == "never" {
		return false
	}
	if mode == "always" {
		return true
	}
	if on, err := strconv.ParseBool(getenv("CLICOLOR_FORCE")); err == nil && on {
		return true
	}
	if on, err := strconv.ParseBool(getenv("CLICOLOR")); err == nil && !on {
		return false
	}
	return IsTerminal(w)
}

// Bytes formats n in binary units: "1023 B", "1.5 KiB", "12 MiB".
func Bytes(n int64) string {
	const unit = 1024
	if n < unit && n > -unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit || m <= -unit; m /= unit {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	if v == float64(int64(v)) || v >= 10 || v <= -10 {
		return fmt.Sprintf("%.0f %ciB", v, "KMGTPE"[exp])
	}
	return fmt.Sprintf("%.1f %ciB", v, "KMGTPE"[exp])
}
