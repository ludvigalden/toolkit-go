package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/update/internal/merge"
	"github.com/ludvigalden/toolkit-go/pkg/update/internal/relnotes"
	"github.com/ludvigalden/toolkit-go/pkg/update/relmapx"
)

// NotesSource supplies release notes for releases newer than a version,
// newest first (ghcli.Source satisfies it).
type NotesSource interface {
	ReleasesNewerThan(ctx context.Context, version string, limit int) ([]Release, error)
}

// Changelog presents a tool's changelog: the relmap export embedded in
// the binary first (offline, always available), then notes of releases
// published after the build, when a remote source is configured.
type Changelog struct {
	// Embedded is the export compiled into the binary; may be nil.
	Embedded *relmapx.Changelog
	// Line is the tool's release line in the export.
	Line string
	// Remote supplies notes for releases newer than the embedded ones;
	// may be nil. ErrUnavailable from it is silent.
	Remote NotesSource
}

// Entry is one release in a changelog view.
type Entry struct {
	Version string `json:"version"`
	Date    string `json:"date,omitempty"`
	Summary string `json:"summary,omitempty"`
	// Notes is free text: the release description for remote entries.
	Notes string `json:"notes,omitempty"`
	// Origin is "embedded" or "remote".
	Origin string `json:"origin"`
	// Release is the full embedded record, nil for remote entries.
	Release *relmapx.Release `json:"release,omitempty"`
}

// View is a collected changelog, newest first.
type View struct {
	Line    string  `json:"line"`
	Entries []Entry `json:"entries"`
	// RemoteUnavailable is set when a remote source was configured but
	// could not run (for example gh is absent).
	RemoteUnavailable bool `json:"remote_unavailable,omitempty"`
	// RemoteError carries a remote failure when embedded entries were
	// still shown; the view is then partial, not failed.
	RemoteError string `json:"remote_error,omitempty"`
}

// CollectOptions narrows a view.
type CollectOptions struct {
	// Since keeps only releases newer than this version.
	Since string
	// Limit caps the number of entries; zero means all.
	Limit int
	// Prereleases includes prerelease entries from the export.
	Prereleases bool
}

// Collect builds the view. A failing remote source is an error only
// when there is nothing embedded to show; otherwise it is recorded in
// View.RemoteError and the embedded entries stand.
func (c *Changelog) Collect(ctx context.Context, opts CollectOptions) (View, error) {
	v := View{Line: c.Line}
	mopts := merge.Options{Since: opts.Since, Limit: opts.Limit, Prereleases: opts.Prereleases}
	var embedded []relmapx.Release
	if c.Embedded != nil {
		embedded = c.Embedded.Releases(c.Line)
	}
	in := make([]merge.Embedded, len(embedded))
	for i, r := range embedded {
		in[i] = merge.Embedded{Version: r.Version, Stable: r.Stable, Ref: i}
	}
	kept, newest := merge.Filter(in, mopts)
	for _, k := range kept {
		r := embedded[k.Ref]
		v.Entries = append(v.Entries, Entry{
			Version: r.Version, Date: r.Date, Summary: r.Summary,
			Origin: "embedded", Release: &r,
		})
	}
	if c.Remote != nil {
		rels, err := c.Remote.ReleasesNewerThan(ctx, merge.Floor(newest, opts.Since), opts.Limit)
		switch {
		case errors.Is(err, ErrUnavailable):
			v.RemoteUnavailable = true
		case err != nil && len(v.Entries) == 0:
			return v, fmt.Errorf("update: remote release notes: %w", err)
		case err != nil:
			v.RemoteError = err.Error()
		}
		remote := make([]Entry, 0, len(rels))
		for _, r := range rels {
			date := ""
			if !r.PublishedAt.IsZero() {
				date = r.PublishedAt.UTC().Format("2006-01-02")
			}
			remote = append(remote, Entry{
				Version: r.Version, Date: date, Summary: r.Name,
				Notes: r.Notes, Origin: "remote",
			})
		}
		v.Entries = append(remote, v.Entries...)
	}
	v.Entries = v.Entries[:merge.Cap(len(v.Entries), opts.Limit)]
	return v, nil
}

// Render writes the view as terminal text.
func (c *Changelog) Render(w io.Writer, v View) error {
	for i, e := range v.Entries {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return fmt.Errorf("update: render changelog: %w", err)
			}
		}
		if e.Release != nil && c.Embedded != nil {
			if err := c.Embedded.RenderRelease(w, *e.Release, relmapx.RenderOptions{}); err != nil {
				return fmt.Errorf("update: render changelog: %w", err)
			}
			continue
		}
		var b strings.Builder
		relnotes.WritePublished(&b, v.Line, e.Version, e.Date, e.Summary, e.Notes)
		if _, err := io.WriteString(w, b.String()); err != nil {
			return fmt.Errorf("update: render changelog: %w", err)
		}
	}
	return nil
}
