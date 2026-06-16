// Package flow runs the version, changelog and update commands behind
// kongcmd: it gathers each payload from the wired capabilities, then
// writes JSON to stdout or a short report to stderr.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/relmapx"
)

// IO carries the command's context and streams.
type IO struct {
	Ctx            context.Context
	Tool           string
	Stdout, Stderr io.Writer
	JSON           bool
}

func (o IO) writeJSON(v any) error {
	enc := json.NewEncoder(o.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

type versionPayload struct {
	Tool     string              `json:"tool"`
	Version  string              `json:"version"`
	Build    *relmapx.BuildState `json:"build,omitempty"`
	Latest   *update.Release     `json:"latest,omitempty"`
	UpToDate *bool               `json:"up_to_date,omitempty"`
}

// Version prints the running version; a non-nil checker adds the
// latest release. An unavailable checker is reported, not failed.
func Version(o IO, version string, build *relmapx.BuildState, check bool, checker update.Checker) error {
	p := versionPayload{Tool: o.Tool, Version: version, Build: build}
	if check {
		if checker == nil {
			_, _ = fmt.Fprintf(o.Stderr, "%s: release check unavailable\n", o.Tool)
		} else if rel, err := checker.Latest(o.Ctx); err == nil {
			upToDate := !update.Newer(rel.Version, version)
			p.Latest, p.UpToDate = &rel, &upToDate
		} else if !update.IsUnavailable(err) {
			return fmt.Errorf("check latest release: %w", err)
		}
	}
	if o.JSON {
		return o.writeJSON(p)
	}
	_, _ = fmt.Fprintf(o.Stdout, "%s %s\n", o.Tool, version)
	if p.Latest != nil {
		if *p.UpToDate {
			_, _ = fmt.Fprintln(o.Stderr, "up to date")
		} else {
			_, _ = fmt.Fprintf(o.Stderr, "%s is available\n", p.Latest.Version)
		}
	}
	return nil
}

// Changelog collects and writes the changelog; a nil changelog is
// reported on stderr and is not an error.
func Changelog(o IO, c *update.Changelog, opts update.CollectOptions) error {
	if c == nil {
		_, _ = fmt.Fprintf(o.Stderr, "%s: no changelog is available in this build\n", o.Tool)
		return nil
	}
	v, err := c.Collect(o.Ctx, opts)
	if err != nil {
		return fmt.Errorf("collect changelog: %w", err)
	}
	if v.RemoteError != "" {
		_, _ = fmt.Fprintf(o.Stderr, "%s: note: newer release notes could not be read: %s\n", o.Tool, v.RemoteError)
	}
	if o.JSON {
		return o.writeJSON(v)
	}
	return c.Render(o.Stdout, v) //nolint:wrapcheck // Render errors carry the update prefix
}

type updatePayload struct {
	From      string          `json:"from"`
	Latest    *update.Release `json:"latest,omitempty"`
	Available bool            `json:"available"`
	Updated   bool            `json:"updated"`
}

// Update checks (checkOnly) or applies the latest release. unavailable
// is returned, wrapped around any cause, when the capability is
// missing or cannot run.
func Update(o IO, from string, checkOnly bool, checker update.Checker, updater update.Updater, unavailable error) error {
	p := updatePayload{From: from}
	wrap := func(err error) error {
		if update.IsUnavailable(err) {
			return fmt.Errorf("%w: %w", unavailable, err)
		}
		return fmt.Errorf("update: %w", err)
	}
	if checkOnly {
		if checker == nil {
			return unavailable
		}
		rel, err := checker.Latest(o.Ctx)
		if err != nil {
			return wrap(err)
		}
		p.Latest, p.Available = &rel, update.Newer(rel.Version, from)
	} else {
		if updater == nil {
			return unavailable
		}
		rel, updated, err := updater.Apply(o.Ctx, from)
		if err != nil {
			return wrap(err)
		}
		p.Latest, p.Updated, p.Available = &rel, updated, updated
	}
	if o.JSON {
		return o.writeJSON(p)
	}
	switch {
	case p.Updated:
		_, _ = fmt.Fprintf(o.Stderr, "%s: updated %s -> %s\n", o.Tool, p.From, p.Latest.Version)
	case p.Available:
		_, _ = fmt.Fprintf(o.Stderr, "%s: %s is available (running %s)\n", o.Tool, p.Latest.Version, p.From)
	default:
		_, _ = fmt.Fprintf(o.Stderr, "%s: %s is the latest release\n", o.Tool, p.From)
	}
	return nil
}
