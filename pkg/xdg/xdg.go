// Package xdg resolves XDG base directories from an injected
// environment, the same way on Linux and macOS (~/.config, not
// ~/Library), and keeps private directories and files owner-only.
package xdg

import (
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/xdg/internal/envpath"
)

// Dirs holds resolved base directories. Every path is absolute and
// clean. RuntimeDir is empty when $XDG_RUNTIME_DIR is unset or invalid;
// use RuntimeDirFor for a directory with a fallback.
type Dirs struct {
	ConfigHome string
	CacheHome  string
	StateHome  string
	DataHome   string
	ConfigDirs []string // system config roots, most important first
	RuntimeDir string
}

// ErrNoHome is returned when $HOME is unset and a default needs it.
var ErrNoHome = envpath.ErrNoHome

// Resolve reads the XDG variables through getenv (nil means os.Getenv).
// Per the spec, empty and relative values are ignored in favour of the
// defaults, all of which need $HOME.
func Resolve(getenv func(string) string) (Dirs, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	var d Dirs
	var err error
	for _, r := range []struct {
		dst       *string
		name, rel string
	}{
		{&d.ConfigHome, "XDG_CONFIG_HOME", ".config"},
		{&d.CacheHome, "XDG_CACHE_HOME", ".cache"},
		{&d.StateHome, "XDG_STATE_HOME", ".local/state"},
		{&d.DataHome, "XDG_DATA_HOME", ".local/share"},
	} {
		if *r.dst, err = envpath.Home(getenv, r.name, r.rel); err != nil {
			return Dirs{}, err //nolint:wrapcheck // ErrNoHome is the documented sentinel
		}
	}
	d.ConfigDirs = envpath.List(getenv, "XDG_CONFIG_DIRS", "/etc/xdg")
	d.RuntimeDir = envpath.Abs(getenv("XDG_RUNTIME_DIR"))
	return d, nil
}
