// Package unit renders systemd user-unit files and parses systemctl show.
package unit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

// defaultOOMScoreAdjust shields small daemons from collateral OOM kills
// while a genuine memory hog stays killable.
const defaultOOMScoreAdjust = -500

// Render returns the byte-stable unit file for spec with env frozen into
// Environment= lines.
func Render(spec supervisor.Spec, env map[string]string) (string, error) {
	tool, err := spec.Tool()
	if err != nil {
		return "", err //nolint:wrapcheck // carries the supervisor prefix
	}
	for _, v := range append([]string{spec.ExePath, spec.WorkingDirectory, spec.StderrLogPath}, spec.Args...) {
		if strings.ContainsAny(v, "\n\r") {
			return "", fmt.Errorf("supervisor: systemd unit: value %q contains a newline", v)
		}
	}
	keys := make([]string, 0, len(env))
	for k, v := range env {
		if strings.ContainsAny(k+v, "\n\r") {
			return "", fmt.Errorf("supervisor: systemd unit: environment %q contains a newline", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=%s supervised service (port %d)\n", tool, spec.Port)
	b.WriteString("After=network.target\n")
	// The default start-limit window marks a unit permanently failed
	// after a burst of collateral kills; 10 per 120s absorbs the burst
	// and still traps a real crash loop.
	b.WriteString("StartLimitIntervalSec=120\nStartLimitBurst=10\n\n")

	b.WriteString("[Service]\nType=exec\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "Environment=%s\n", quoteEnv(k, env[k]))
	}
	b.WriteString("ExecStart=" + quoteArg(spec.ExePath))
	for _, a := range spec.Args {
		b.WriteString(" " + quoteArg(a))
	}
	b.WriteString("\n")
	if spec.WorkingDirectory != "" {
		// Path settings take the whole line value, so an unquoted path
		// may contain spaces; quoting is kept literally here. Specifier
		// expansion does not apply (verified live: a transient unit
		// chdir'd into a literal %n directory).
		fmt.Fprintf(&b, "WorkingDirectory=%s\n", spec.WorkingDirectory)
	}
	// Exponential restart backoff needs systemd 249; older versions
	// ignore the keys and keep the fixed RestartSec.
	b.WriteString("TimeoutStopSec=35\nRestart=always\nRestartSec=2\nRestartSteps=5\nRestartMaxDelaySec=60\n")
	oom := defaultOOMScoreAdjust
	if spec.OOMScoreAdjust != nil {
		oom = *spec.OOMScoreAdjust
	}
	fmt.Fprintf(&b, "OOMScoreAdjust=%d\n", oom)
	if spec.StderrLogPath != "" {
		// append: takes the whole line value; quoting is kept literal.
		// Specifier expansion does not apply to append: paths (verified
		// live: a transient unit created the literal %-name on disk), so
		// a log path containing % is emitted raw.
		fmt.Fprintf(&b, "StandardOutput=append:%s\nStandardError=append:%s\n", spec.StderrLogPath, spec.StderrLogPath)
	} else {
		b.WriteString("StandardOutput=journal\nStandardError=journal\n")
	}
	b.WriteString("\n[Install]\nWantedBy=default.target\n")
	return b.String(), nil
}

// ParseShow parses `systemctl show --property=...` key=value output.
func ParseShow(s string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// quoteArg renders s as one systemd command-line word: whitespace, quote,
// backslash, dollar or percent trigger quoting with the matching escapes,
// since systemd word-splits ExecStart and expands $ and % specifiers.
func quoteArg(s string) string {
	if !strings.ContainsAny(s, " \t\"\\$%") {
		return s
	}
	return `"` + escape(s) + `"`
}

// quoteEnv renders one Environment= assignment. systemd splits it on
// unquoted whitespace (a spaced home directory would silently truncate)
// and expands % specifiers, so % is doubled; $ stays literal here,
// unlike in ExecStart.
func quoteEnv(key, value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	if !strings.ContainsAny(value, " \t\"\\") {
		return key + "=" + value
	}
	var b strings.Builder
	b.Grow(len(key) + len(value) + 3)
	b.WriteString(`"` + key + `=`)
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String() + `"`
}

func escape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '$':
			b.WriteString("$$")
		case '%':
			b.WriteString("%%")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// StartFailure classifies one property sample taken after `systemctl
// start` as terminal ("" means the start may still be in flight). Under
// Restart=always only a nonzero exit marks auto-restart as a crash.
func StartFailure(p map[string]string) string {
	active, sub, result, status := p["ActiveState"], p["SubState"], p["Result"], p["ExecMainStatus"]
	switch {
	case active == "failed":
		return fmt.Sprintf("failed (Result=%s ExecMainStatus=%s)", result, status)
	case result == "start-limit-hit":
		return fmt.Sprintf("hit the start rate limit (ExecMainStatus=%s)", status)
	case sub == "auto-restart" && status != "" && status != "0" && status != "-1":
		return fmt.Sprintf("exited %s and is cycling under Restart=always", status)
	case active == "inactive":
		// systemctl start returns after the job completes, so inactive
		// here means a concurrent stop, never a slow start.
		return "went inactive immediately after the start job"
	}
	return ""
}
