// Package update composes release awareness from small capabilities:
// a Checker finds the latest release, an Updater replaces the binary,
// and Notice and Changelog combine them for the common surfaces.
package update

import (
	"context"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/update/internal/semver"
)

// ErrUnavailable means a capability cannot run here (no gh binary, no
// release source); surfaces render nothing rather than fail. Its
// errkind is Unavailable, so returning it exits exitcode.Unavailable.
var ErrUnavailable error = errkind.New(errkind.Unavailable, "update: unavailable")

// Release is a published release as a source reports it.
type Release struct {
	Tag         string    `json:"tag"`
	Version     string    `json:"version"`
	Name        string    `json:"name,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	URL         string    `json:"url,omitempty"`
	PublishedAt time.Time `json:"published_at,omitzero"`
	Prerelease  bool      `json:"prerelease,omitempty"`
}

// Checker reports the latest published release.
type Checker interface {
	Latest(ctx context.Context) (Release, error)
}

// Lister reports published releases, newest first.
type Lister interface {
	Releases(ctx context.Context, limit int) ([]Release, error)
}

// Updater replaces the running binary with the latest release when it
// is newer than current, returning the release applied (or the latest
// one when no update was needed) and whether the binary changed.
type Updater interface {
	Apply(ctx context.Context, current string) (rel Release, updated bool, err error)
}

// Newer reports whether candidate is a strictly newer version than
// current. Leading "v" is accepted on either side.
func Newer(candidate, current string) bool {
	return semver.CompareStrings(candidate, current) > 0
}

// VersionFromTag strips a leading "v" so tags compare as versions.
func VersionFromTag(tag string) string {
	if len(tag) > 1 && tag[0] == 'v' && tag[1] >= '0' && tag[1] <= '9' {
		return tag[1:]
	}
	return tag
}
