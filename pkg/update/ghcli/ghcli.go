// Package ghcli reads GitHub releases through the gh CLI, so private
// repositories work with the user's own login. Without gh on PATH every
// call returns update.ErrUnavailable; there is no HTTP fallback.
package ghcli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/internal/gh"
)

// Runner executes gh with args and returns its stdout. It is the seam
// tests replace.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ExecRunner runs the gh binary found on PATH.
type ExecRunner struct {
	// Path overrides the binary; empty means look up "gh" on PATH at
	// each call, so installing gh mid-session is picked up.
	Path string
}

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := gh.Exec(ctx, r.Path, args...)
	if errors.Is(err, gh.ErrNotFound) {
		return nil, update.ErrUnavailable
	}
	return out, err //nolint:wrapcheck // gh.Exec names the subcommand and carries gh's stderr
}

// Available reports whether gh is on PATH.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// Source reads releases of one repository. It satisfies update.Checker
// and update.Lister.
type Source struct {
	repo   string
	runner Runner
	pre    bool
}

// Option configures a Source.
type Option func(*Source)

// WithRunner replaces the gh runner.
func WithRunner(r Runner) Option { return func(s *Source) { s.runner = r } }

// WithPrereleases makes Latest consider prereleases.
func WithPrereleases() Option { return func(s *Source) { s.pre = true } }

// New returns a Source for "owner/name".
func New(repo string, opts ...Option) *Source {
	s := &Source{repo: repo, runner: ExecRunner{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

func convert(g gh.Release) update.Release {
	return update.Release{
		Tag:         g.TagName,
		Version:     update.VersionFromTag(g.TagName),
		Name:        g.Name,
		Notes:       g.Body,
		URL:         g.URL,
		PublishedAt: g.PublishedAt,
		Prerelease:  g.IsPrerelease,
	}
}

// Latest returns the latest release. Without WithPrereleases it is the
// release GitHub marks latest; with it, the newest release of any kind.
func (s *Source) Latest(ctx context.Context) (update.Release, error) {
	if s.pre {
		rels, err := s.Releases(ctx, 20)
		if err != nil {
			return update.Release{}, err
		}
		if len(rels) == 0 {
			return update.Release{}, fmt.Errorf("ghcli: %s has no releases: %w", s.repo, update.ErrUnavailable)
		}
		return rels[0], nil
	}
	return s.View(ctx, "")
}

// View returns one release by tag; an empty tag means the latest.
func (s *Source) View(ctx context.Context, tag string) (update.Release, error) {
	args := []string{"release", "view"}
	if tag != "" {
		args = append(args, tag)
	}
	args = append(args, "--repo", s.repo, "--json", gh.ViewFields)
	out, err := s.runner.Run(ctx, args...)
	if err != nil {
		return update.Release{}, err
	}
	g, err := gh.DecodeRelease(out)
	if err != nil {
		return update.Release{}, err //nolint:wrapcheck // decode errors carry the ghcli prefix
	}
	return convert(g), nil
}

// Releases lists up to limit non-draft releases, newest first, without
// notes: gh's list output carries no body (ReleasesNewerThan fetches
// them per release).
func (s *Source) Releases(ctx context.Context, limit int) ([]update.Release, error) {
	if limit <= 0 {
		limit = 30
	}
	out, err := s.runner.Run(ctx, "release", "list", "--repo", s.repo,
		"--limit", strconv.Itoa(limit), "--exclude-drafts",
		"--json", gh.ListFields)
	if err != nil {
		return nil, err
	}
	gs, err := gh.DecodeList(out)
	if err != nil {
		return nil, err //nolint:wrapcheck // decode errors carry the ghcli prefix
	}
	rels := make([]update.Release, 0, len(gs))
	for _, g := range gs {
		if g.IsDraft || (!s.pre && g.IsPrerelease) {
			continue
		}
		rels = append(rels, convert(g))
	}
	return rels, nil
}

// ReleasesNewerThan returns the releases newer than version, newest
// first, each with its notes (the release description).
func (s *Source) ReleasesNewerThan(ctx context.Context, version string, limit int) ([]update.Release, error) {
	rels, err := s.Releases(ctx, limit)
	if err != nil {
		return nil, err
	}
	var out []update.Release
	for _, r := range rels {
		if !update.Newer(r.Version, version) {
			continue
		}
		full, err := s.View(ctx, r.Tag)
		if err != nil {
			return nil, err
		}
		out = append(out, full)
	}
	return out, nil
}

// IsUnavailable reports whether err means gh is absent.
func IsUnavailable(err error) bool { return errors.Is(err, update.ErrUnavailable) }
