package relmapx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"

	"github.com/ludvigalden/toolkit-go/pkg/update/internal/exportio"
	"github.com/ludvigalden/toolkit-go/pkg/update/internal/semver"
)

// ErrSchema is wrapped by every error for an export whose schema this
// package does not understand.
var ErrSchema = errors.New("relmapx: unsupported export schema")

// Changelog is a loaded relmap export: the ndjson records plus the
// optional index and metadata files.
type Changelog struct {
	header   Header
	releases []Release
	changes  []Change
	byID     map[string]int
	index    *Index
	metadata *Metadata
}

// Load reads an export directory from fsys. changelog.ndjson is
// required; the index and metadata files are read when present, so a
// tool may embed only the ndjson.
func Load(fsys fs.FS, dir string) (*Changelog, error) {
	if dir == "" {
		dir = "."
	}
	raw, err := fs.ReadFile(fsys, path.Join(dir, NDJSONFile))
	if err != nil {
		return nil, fmt.Errorf("relmapx: read %s: %w", NDJSONFile, err)
	}
	c, err := ParseNDJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	var idx Index
	if ok, err := exportio.ReadOptional(fsys, path.Join(dir, IndexFile), IndexFile, &idx); err != nil {
		return nil, err //nolint:wrapcheck // exportio errors already carry the relmapx prefix and file
	} else if ok {
		if idx.Schema != SchemaVersion {
			return nil, fmt.Errorf("%w: %s has schema %d, want %d", ErrSchema, IndexFile, idx.Schema, SchemaVersion)
		}
		c.index = &idx
	}
	var md Metadata
	if ok, err := exportio.ReadOptional(fsys, path.Join(dir, MetadataFile), MetadataFile, &md); err != nil {
		return nil, err //nolint:wrapcheck // exportio errors already carry the relmapx prefix and file
	} else if ok {
		c.metadata = &md
	}
	return c, nil
}

// ParseNDJSON decodes changelog.ndjson. The first record must be the
// header; record kinds this package does not know are skipped.
func ParseNDJSON(r io.Reader) (*Changelog, error) {
	c := &Changelog{byID: map[string]int{}}
	var schemaErr error
	err := exportio.Scan(r, NDJSONFile, func(kind string, b []byte) error {
		switch kind {
		case "header":
			if err := json.Unmarshal(b, &c.header); err != nil {
				return err //nolint:wrapcheck // Scan adds file and line
			}
			if c.header.Schema != SchemaVersion {
				schemaErr = fmt.Errorf("%w: %s has schema %d, want %d", ErrSchema, NDJSONFile, c.header.Schema, SchemaVersion)
				return schemaErr
			}
		case "release":
			var rel Release
			if err := json.Unmarshal(b, &rel); err != nil {
				return err //nolint:wrapcheck // Scan adds file and line
			}
			c.byID[rel.ID] = len(c.releases)
			c.releases = append(c.releases, rel)
		case "change":
			var ch Change
			if err := json.Unmarshal(b, &ch); err != nil {
				return err //nolint:wrapcheck // Scan adds file and line
			}
			c.changes = append(c.changes, ch)
		}
		return nil
	})
	if schemaErr != nil {
		return nil, schemaErr
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // exportio errors already carry the relmapx prefix, file and line
	}
	return c, nil
}

// Header returns the export header.
func (c *Changelog) Header() Header { return c.header }

// Index returns the parsed index, or nil when it was not exported.
func (c *Changelog) Index() *Index { return c.index }

// Metadata returns the parsed metadata, or nil when it was not exported.
func (c *Changelog) Metadata() *Metadata { return c.metadata }

// Lines returns the release line names, sorted.
func (c *Changelog) Lines() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range c.releases {
		if !seen[r.Line] {
			seen[r.Line] = true
			out = append(out, r.Line)
		}
	}
	sort.Strings(out)
	return out
}

// Release returns the release with the "<line>@<version>" id.
func (c *Changelog) Release(id string) (Release, bool) {
	i, ok := c.byID[id]
	if !ok {
		return Release{}, false
	}
	return c.releases[i], true
}

// Releases returns a line's releases, newest version first.
func (c *Changelog) Releases(line string) []Release {
	var out []Release
	for _, r := range c.releases {
		if r.Line == line {
			out = append(out, r)
		}
	}
	sortNewestFirst(out)
	return out
}

// Current returns the line's current release: the one relmap marks
// current, else the newest stable one.
func (c *Changelog) Current(line string) (Release, bool) {
	rels := c.Releases(line)
	for _, r := range rels {
		if r.Current {
			return r, true
		}
	}
	for _, r := range rels {
		if r.Stable {
			return r, true
		}
	}
	return Release{}, false
}

// Newer returns the line's releases strictly newer than version, newest
// first. Prereleases are included only when includePre is set, so a
// stable build is not nagged about release candidates.
func (c *Changelog) Newer(line, version string, includePre bool) []Release {
	var out []Release
	for _, r := range c.Releases(line) {
		if !includePre && !r.Stable {
			continue
		}
		if semver.CompareStrings(r.Version, version) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// Changes returns the change records a release includes, in export
// order. An empty releaseID returns the pending changes.
func (c *Changelog) Changes(releaseID string) []Change {
	var out []Change
	for _, ch := range c.changes {
		if ch.Release == releaseID {
			out = append(out, ch)
		}
	}
	return out
}

// ChangesFor returns the changes reaching component, in export order.
func (c *Changelog) ChangesFor(component string) []Change {
	var out []Change
	for _, ch := range c.changes {
		for _, a := range ch.Affected {
			if a.Component == component {
				out = append(out, ch)
				break
			}
		}
	}
	return out
}

func sortNewestFirst(rels []Release) {
	sort.SliceStable(rels, func(i, j int) bool {
		if c := semver.CompareStrings(rels[i].Version, rels[j].Version); c != 0 {
			return c > 0
		}
		return rels[i].Date > rels[j].Date
	})
}
