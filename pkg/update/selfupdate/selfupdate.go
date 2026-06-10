// Package selfupdate replaces the running binary with a newer release
// via go-selfupdate. It is the module's one heavy package; nothing else
// here imports it, so only tools that use it pay for it.
package selfupdate

import (
	"context"
	"errors"
	"fmt"

	gsu "github.com/creativeprojects/go-selfupdate"

	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/selfupdate/internal/gsux"
)

// ChecksumsFile is the conventional checksum asset name: one
// "<sha256>  <asset>" line per release asset.
const ChecksumsFile = "checksums.txt"

// Config configures a Self updater.
type Config struct {
	// Repo is "owner/name".
	Repo string
	// Source supplies releases; nil means GitHub's API (honouring
	// GITHUB_TOKEN). Inject a custom source for other hosts or tests.
	Source gsu.Source
	// Validator verifies downloads; nil means checksums.txt validation.
	// Set SkipValidation to opt out explicitly.
	Validator      gsu.Validator
	SkipValidation bool
	// Prerelease lets prereleases count as the latest release.
	Prerelease bool
	// Executable is the binary to replace; empty means the running one.
	Executable string
}

// Self implements update.Checker and update.Updater.
type Self struct {
	cfg Config
	up  *gsu.Updater
}

// New builds a Self from cfg.
func New(cfg Config) (*Self, error) {
	if cfg.Repo == "" {
		return nil, errors.New("selfupdate: Config.Repo is required")
	}
	up, err := gsux.NewUpdater(cfg.Source, cfg.Validator, cfg.SkipValidation, cfg.Prerelease, ChecksumsFile)
	if err != nil {
		return nil, err //nolint:wrapcheck // gsux errors carry the selfupdate prefix
	}
	return &Self{cfg: cfg, up: up}, nil
}

// NewHTTP builds a Self over a plain HTTP release server laid out as
// go-selfupdate's HttpSource expects (<base>/<owner>/<repo>/manifest.yaml).
func NewHTTP(baseURL string, cfg Config) (*Self, error) {
	src, err := gsu.NewHttpSource(gsu.HttpConfig{BaseURL: baseURL})
	if err != nil {
		return nil, fmt.Errorf("selfupdate: http source: %w", err)
	}
	cfg.Source = src
	return New(cfg)
}

func (s *Self) detect(ctx context.Context) (*gsu.Release, error) {
	rel, err := gsux.Detect(ctx, s.up, s.cfg.Repo)
	if errors.Is(err, gsux.ErrNoAsset) {
		return nil, fmt.Errorf("selfupdate: no release of %s has an asset for this platform: %w", s.cfg.Repo, update.ErrUnavailable)
	}
	return rel, err //nolint:wrapcheck // gsux errors carry the selfupdate prefix and repo
}

func convert(r *gsu.Release) update.Release {
	return update.Release{
		Tag:         "v" + r.Version(),
		Version:     r.Version(),
		Name:        r.Name,
		Notes:       r.ReleaseNotes,
		URL:         r.URL,
		PublishedAt: r.PublishedAt,
		Prerelease:  r.Prerelease,
	}
}

// Latest implements update.Checker: the newest release with an asset
// for this OS and architecture.
func (s *Self) Latest(ctx context.Context) (update.Release, error) {
	r, err := s.detect(ctx)
	if err != nil {
		return update.Release{}, err
	}
	return convert(r), nil
}

// Apply implements update.Updater. It downloads and validates the
// latest asset and replaces the executable only when the release is
// newer than current.
func (s *Self) Apply(ctx context.Context, current string) (update.Release, bool, error) {
	r, err := s.detect(ctx)
	if err != nil {
		return update.Release{}, false, err
	}
	rel := convert(r)
	if !update.Newer(rel.Version, current) {
		return rel, false, nil
	}
	exe, err := gsux.Executable(s.cfg.Executable)
	if err != nil {
		return rel, false, err //nolint:wrapcheck // gsux errors carry the selfupdate prefix
	}
	if err := s.up.UpdateTo(ctx, r, exe); err != nil {
		return rel, false, fmt.Errorf("selfupdate: replace %s with %s: %w", exe, rel.Version, err)
	}
	return rel, true, nil
}
