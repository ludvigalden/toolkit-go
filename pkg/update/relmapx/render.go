package relmapx

import (
	"fmt"
	"io"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/update/internal/relnotes"
)

// RenderOptions shapes terminal rendering.
type RenderOptions struct {
	// Limit caps the number of releases rendered; zero means all.
	Limit int
	// Changes adds each release's change records below its sections.
	Changes bool
}

// RenderRelease writes one release as plain text for a terminal.
func (c *Changelog) RenderRelease(w io.Writer, r Release, opts RenderOptions) error {
	cur := relnotes.Curated{
		Line: r.Line, Version: r.Version, Date: r.Date, Channel: r.Channel,
		Stable: r.Stable, Summary: r.Summary, Paragraph: r.Paragraph,
	}
	for _, s := range r.Sections {
		sec := relnotes.Section{Category: s.Category}
		for _, it := range s.Items {
			sec.Items = append(sec.Items, it.Text)
		}
		cur.Sections = append(cur.Sections, sec)
	}
	if opts.Changes {
		for _, ch := range c.Changes(r.ID) {
			cur.Changes = append(cur.Changes, relnotes.Change{Category: ch.Category, Summary: ch.Summary, Migration: ch.Migration})
		}
	}
	var b strings.Builder
	relnotes.WriteCurated(&b, cur)
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("relmapx: render: %w", err)
	}
	return nil
}

// Render writes releases newest first, separated by blank lines.
func (c *Changelog) Render(w io.Writer, rels []Release, opts RenderOptions) error {
	for i, r := range rels {
		if opts.Limit > 0 && i >= opts.Limit {
			break
		}
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return fmt.Errorf("relmapx: render: %w", err)
			}
		}
		if err := c.RenderRelease(w, r, opts); err != nil {
			return err
		}
	}
	return nil
}
