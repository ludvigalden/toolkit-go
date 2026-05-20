// Package relnotes renders release notes as plain terminal text: a
// curated relmap release with its sections, or a published release
// with free-text notes.
package relnotes

import (
	"fmt"
	"strings"
)

// Section is one category of curated items.
type Section struct {
	Category string
	Items    []string
}

// Change is one change record listed under a release.
type Change struct {
	Category, Summary, Migration string
}

// Curated is a relmap release as the renderer needs it.
type Curated struct {
	Line, Version, Date, Channel string
	Stable                       bool
	Summary, Paragraph           string
	Sections                     []Section
	Changes                      []Change
}

// WriteCurated appends the release block to b.
func WriteCurated(b *strings.Builder, r Curated) {
	fmt.Fprintf(b, "%s %s (%s)", r.Line, r.Version, r.Date)
	if !r.Stable {
		fmt.Fprintf(b, " [%s]", r.Channel)
	}
	b.WriteString("\n")
	if r.Summary != "" {
		fmt.Fprintf(b, "  %s\n", r.Summary)
	}
	if r.Paragraph != "" {
		fmt.Fprintf(b, "\n%s\n", indent(wrap(r.Paragraph, 72), "  "))
	}
	for _, s := range r.Sections {
		fmt.Fprintf(b, "\n  %s\n", title(s.Category))
		for _, it := range s.Items {
			fmt.Fprintf(b, "    - %s\n", it)
		}
	}
	for _, ch := range r.Changes {
		fmt.Fprintf(b, "    * [%s] %s\n", ch.Category, ch.Summary)
		if ch.Migration != "" {
			fmt.Fprintf(b, "      migration: %s\n", ch.Migration)
		}
	}
}

// WritePublished appends a published release: a header marked
// [published], its title when it says more than the version, then
// the notes indented.
func WritePublished(b *strings.Builder, line, version, date, name, notes string) {
	fmt.Fprintf(b, "%s %s", line, version)
	if date != "" {
		fmt.Fprintf(b, " (%s)", date)
	}
	b.WriteString(" [published]\n")
	if name != "" && name != version && name != "v"+version {
		fmt.Fprintf(b, "  %s\n", name)
	}
	if notes = strings.TrimSpace(notes); notes != "" {
		for _, l := range strings.Split(notes, "\n") {
			fmt.Fprintf(b, "  %s\n", strings.TrimRight(l, "\r "))
		}
	}
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

func wrap(s string, width int) string {
	var b strings.Builder
	n := 0
	for _, word := range strings.Fields(s) {
		if n > 0 && n+1+len(word) > width {
			b.WriteString("\n")
			n = 0
		} else if n > 0 {
			b.WriteString(" ")
			n++
		}
		b.WriteString(word)
		n += len(word)
	}
	return b.String()
}
