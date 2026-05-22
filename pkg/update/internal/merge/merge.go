// Package merge combines a changelog's embedded releases with notes of
// releases published after the build: it filters the embedded ones,
// chooses the version floor to ask the remote for, and orders the result.
package merge

import "github.com/ludvigalden/toolkit-go/pkg/update/internal/semver"

// Embedded is one release from the binary's export; Ref lets the
// caller find its full record again.
type Embedded struct {
	Version string
	Stable  bool
	Ref     int
}

// Options narrows the merge.
type Options struct {
	Since       string
	Limit       int
	Prereleases bool
}

// Filter keeps the embedded releases (newest first) the options admit
// and returns the newest embedded version, admitted or not.
func Filter(rels []Embedded, opts Options) (kept []Embedded, newest string) {
	for _, r := range rels {
		if newest == "" {
			newest = r.Version
		}
		if !opts.Prereleases && !r.Stable {
			continue
		}
		if opts.Since != "" && semver.CompareStrings(r.Version, opts.Since) <= 0 {
			continue
		}
		kept = append(kept, r)
	}
	return kept, newest
}

// Floor is the version the remote source is asked to exceed: the
// newest embedded release, raised to Since when that is newer, or
// "0.0.0" when neither is known.
func Floor(newestEmbedded, since string) string {
	floor := newestEmbedded
	if floor == "" || (since != "" && semver.CompareStrings(since, floor) > 0) {
		floor = since
	}
	if floor == "" {
		floor = "0.0.0"
	}
	return floor
}

// Cap truncates n entries to limit; zero means no limit.
func Cap(n, limit int) int {
	if limit > 0 && n > limit {
		return limit
	}
	return n
}
