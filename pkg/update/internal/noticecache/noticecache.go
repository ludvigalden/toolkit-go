// Package noticecache persists the update notice's last check result
// as a small JSON file under the tool's cache directory, and resolves
// the opt-out and terminal checks the notice needs.
package noticecache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is the cache file inside the tool's cache directory.
const FileName = "update-check.json"

// Entry is one remembered check.
type Entry struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	URL       string    `json:"url,omitempty"`
}

// Fresh reports whether e can be reused at now.
func (e Entry) Fresh(now time.Time, ttl time.Duration) bool {
	return e.Latest != "" && now.Sub(e.CheckedAt) < ttl
}

// Path returns the cache file for tool under dir; "" when dir is
// empty, so a caller that cannot resolve a cache directory simply
// skips caching.
func Path(dir, tool string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, FileName)
}

// Read returns the cached entry; a missing or corrupt file yields the
// zero Entry, which is never fresh.
func Read(path string) Entry {
	var e Entry
	if path == "" {
		return e
	}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &e) //nolint:errcheck // a corrupt cache is refreshed by the caller
	}
	return e
}

// Write stores e atomically (temp file, then rename). It is best
// effort: a notice must never fail a command over its cache.
func Write(path string, e Entry) {
	if path == "" {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path) //nolint:errcheck // best-effort cache
}

// OptOutVar is the variable that disables tool's notice:
// <TOOL>_NO_UPDATE_NOTIFIER with non-alphanumerics as underscores.
func OptOutVar(tool string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, tool) + "_NO_UPDATE_NOTIFIER"
}

// IsTerminal reports whether f is a character device.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
