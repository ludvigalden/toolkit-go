package relmapx

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/update/internal/semver"
)

// BuildState is the output of `relmap version <component> --json`,
// saved at build time and embedded so a binary knows the version it
// presents and whether that build could be judged clean.
type BuildState struct {
	Schema    int        `json:"schema"`
	Query     string     `json:"query"`
	Component string     `json:"component"`
	Line      string     `json:"line"`
	Channel   string     `json:"channel"`
	Base      string     `json:"base"`
	BaseFrom  string     `json:"base_from"`
	Presented string     `json:"presented"`
	State     BuildFlags `json:"state"`
}

// ParseBuildState decodes `relmap version --json` output.
func ParseBuildState(b []byte) (BuildState, error) {
	var s BuildState
	if err := json.Unmarshal(b, &s); err != nil {
		return BuildState{}, fmt.Errorf("relmapx: decode build state: %w", err)
	}
	if s.Schema != SchemaVersion {
		return BuildState{}, fmt.Errorf("%w: build state has schema %d, want %d", ErrSchema, s.Schema, SchemaVersion)
	}
	if s.Presented == "" {
		return BuildState{}, fmt.Errorf("relmapx: build state has no presented version")
	}
	return s, nil
}

// LoadBuildState reads and decodes a saved build state from fsys.
func LoadBuildState(fsys fs.FS, name string) (BuildState, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return BuildState{}, fmt.Errorf("relmapx: read build state: %w", err)
	}
	return ParseBuildState(b)
}

// Clean reports whether the build presents its base version unmarked.
func (s BuildState) Clean() bool { return !s.State.Unknown && !s.State.Dirty }

// Version returns the presented version without build metadata
// ("2.7.0+unknown.abc" becomes "2.7.0"), the form release comparisons
// need.
func (s BuildState) Version() string {
	v, _, _ := strings.Cut(s.Presented, "+")
	return v
}

// OlderThan reports whether this build is older than version. A build
// with unknown state still compares by its base, so notices keep
// working on development builds.
func (s BuildState) OlderThan(version string) bool {
	return semver.CompareStrings(s.Version(), version) < 0
}
