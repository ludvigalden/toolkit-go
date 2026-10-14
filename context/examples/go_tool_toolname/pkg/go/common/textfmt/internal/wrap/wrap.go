// Package wrap implements textfmt's word wrapping.
package wrap

import "strings"

// Words greedily packs the words of s into lines of at most width runes.
func Words(s string, width int) []string {
	var lines []string
	var cur strings.Builder
	for _, w := range strings.Fields(s) {
		if cur.Len() > 0 && len([]rune(cur.String()))+1+len([]rune(w)) > width {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
