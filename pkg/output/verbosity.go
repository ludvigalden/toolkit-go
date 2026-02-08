package output

import (
	"fmt"
	"strconv"
	"strings"
)

// Verbosity is the 0-10 diagnostic threshold. Results are never gated;
// each diagnostic kind needs a minimum level. The zero value is silent.
type Verbosity int

// The named verbosity levels.
const (
	VerbositySilent        Verbosity = 0
	VerbosityQuiet         Verbosity = 1
	VerbosityFatal         Verbosity = 2
	VerbosityError         Verbosity = 3
	VerbosityWarn          Verbosity = 4
	VerbosityInfo          Verbosity = 5
	VerbosityVerbose       Verbosity = 6
	VerbosityDebug         Verbosity = 7
	VerbosityTrace         Verbosity = 8
	VerbosityDetailedTrace Verbosity = 9
	VerbosityMax           Verbosity = 10
)

var verbosityNames = [...]string{
	"silent", "quiet", "fatal", "error", "warn", "info",
	"verbose", "debug", "trace", "detailed-trace", "max",
}

// String returns the level name.
func (v Verbosity) String() string {
	if v >= 0 && int(v) < len(verbosityNames) {
		return verbosityNames[v]
	}
	return "verbosity(" + strconv.Itoa(int(v)) + ")"
}

// ParseVerbosity accepts a level name (case-insensitive) or 0-10.
func ParseVerbosity(s string) (Verbosity, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > int(VerbosityMax) {
			return 0, fmt.Errorf("invalid verbosity %q: must be 0-10", s)
		}
		return Verbosity(n), nil
	}
	for i, name := range verbosityNames {
		if name == s {
			return Verbosity(i), nil
		}
	}
	return 0, fmt.Errorf("invalid verbosity %q: must be a level name (silent..max) or 0-10", s)
}

// admits reports whether a diagnostic of kind k is shown. A failure
// given to Fail survives down to VerbosityQuiet.
func (v Verbosity) admits(k Kind, fatal bool) bool {
	if fatal {
		return v >= VerbosityQuiet
	}
	switch k {
	case KindError:
		return v >= VerbosityError
	case KindWarn:
		return v >= VerbosityWarn
	case KindDebug:
		return v >= VerbosityDebug
	}
	return v >= VerbosityInfo
}
