// Package envpath reads XDG-style path variables through an injected
// getenv: empty and relative values are ignored in favour of defaults.
package envpath

import (
	"errors"
	"path/filepath"
)

// ErrNoHome is returned when $HOME is unset and a default needs it.
var ErrNoHome = errors.New("xdg: $HOME is not set")

// Abs returns v cleaned when it is absolute, else "".
func Abs(v string) string {
	if v == "" || !filepath.IsAbs(v) {
		return ""
	}
	return filepath.Clean(v)
}

// Home resolves one single-path variable, falling back to $HOME/rel.
func Home(getenv func(string) string, name, rel string) (string, error) {
	if v := Abs(getenv(name)); v != "" {
		return v, nil
	}
	h := Abs(getenv("HOME"))
	if h == "" {
		return "", ErrNoHome
	}
	return filepath.Join(h, rel), nil
}

// List splits a path-list variable, keeping absolute entries and
// returning def when none remain.
func List(getenv func(string) string, name string, def ...string) []string {
	var dirs []string
	for _, e := range filepath.SplitList(getenv(name)) {
		if v := Abs(e); v != "" {
			dirs = append(dirs, v)
		}
	}
	if len(dirs) == 0 {
		return def
	}
	return dirs
}
