// Package noise strips lines that a wrapping shell's init (zsh zle
// warnings) or a systemd-run scope launcher writes to stderr ahead of
// a subprocess's real output.
package noise

import (
	"bytes"
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)can't change option:\s*zle`),
	regexp.MustCompile(`(?i)^zle\b`),
	regexp.MustCompile(`(?i)^running as unit:`),
	regexp.MustCompile(`(?i)\binvocation ID:\s*[0-9a-f]+$`),
	regexp.MustCompile(`(?i)^scope command line contains environment variable\b`),
}

// Strip drops blank and noise lines, returning nil when none remain.
func Strip(stderr []byte) []byte {
	var kept [][]byte
	for line := range bytes.SplitSeq(stderr, []byte{'\n'}) {
		s := strings.TrimSpace(string(line))
		if s == "" || isNoise(s) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return nil
	}
	return bytes.Join(kept, []byte{'\n'})
}

func isNoise(s string) bool {
	for _, re := range patterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
