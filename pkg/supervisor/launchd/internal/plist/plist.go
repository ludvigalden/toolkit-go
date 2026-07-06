// Package plist renders launchd LaunchAgent property lists.
package plist

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

// Agent is the subset of LaunchAgent keys the supervisor emits.
type Agent struct {
	Label            string
	Program          []string
	Env              map[string]string
	WorkingDirectory string
	LogPath          string
}

// Render returns the byte-stable plist for a. Every string is XML-escaped:
// an unescaped & or < makes launchctl reject or truncate the plist.
func Render(a Agent) (string, error) {
	if a.Label == "" || len(a.Program) == 0 || a.Program[0] == "" {
		return "", fmt.Errorf("supervisor: plist: label and program are required")
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	keyString(&b, "Label", a.Label)
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, arg := range a.Program {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", esc(arg))
	}
	b.WriteString("\t</array>\n")
	if len(a.Env) > 0 {
		keys := make([]string, 0, len(a.Env))
		for k := range a.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", esc(k), esc(a.Env[k]))
		}
		b.WriteString("\t</dict>\n")
	}
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>KeepAlive</key>\n\t<true/>\n")
	if a.WorkingDirectory != "" {
		keyString(&b, "WorkingDirectory", a.WorkingDirectory)
	}
	if a.LogPath != "" {
		keyString(&b, "StandardOutPath", a.LogPath)
		keyString(&b, "StandardErrorPath", a.LogPath)
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String(), nil
}

func keyString(b *strings.Builder, key, val string) {
	fmt.Fprintf(b, "\t<key>%s</key>\n\t<string>%s</string>\n", key, esc(val))
}

func esc(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s)) //nolint:errcheck // bytes.Buffer never fails
	return buf.String()
}

// State returns the `state = ` value from launchctl print output.
func State(out string) string { return field(out, "state =") }

// PID returns the `pid = ` value from launchctl print output, 0 if absent.
func PID(out string) int {
	var pid int
	if _, err := fmt.Sscanf(field(out, "pid ="), "%d", &pid); err != nil {
		return 0
	}
	return pid
}

func field(out, prefix string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
