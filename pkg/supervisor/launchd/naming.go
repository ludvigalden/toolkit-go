// Package launchd is the macOS launchd per-user agent supervisor backend.
// Naming and log paths are portable; the backend itself is darwin-only.
package launchd

import (
	"fmt"
	"path/filepath"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
	"github.com/ludvigalden/toolkit-go/pkg/xdg"
)

// Mode is the Status.Mode this backend reports.
const Mode = "launchd"

// SystemPATH follows the executable's directory in every agent's PATH; it
// is the stock per-user launchd session PATH.
const SystemPATH = "/usr/bin:/bin:/usr/sbin:/sbin"

// Label returns spec's agent label, its service identity.
func Label(spec supervisor.Spec) (string, error) {
	return spec.Identity() //nolint:wrapcheck // carries the supervisor prefix
}

// PlistFileName returns spec's plist file name, <label>.plist.
func PlistFileName(spec supervisor.Spec) (string, error) {
	label, err := Label(spec)
	if err != nil {
		return "", err
	}
	return label + ".plist", nil
}

// DefaultLogDir returns $XDG_STATE_HOME/<app>/logs, read through getenv
// (nil: os.Getenv): persistent across reboots yet disposable.
func DefaultLogDir(app string, getenv func(string) string) (string, error) {
	if app == "" || filepath.Base(app) != app {
		return "", fmt.Errorf("supervisor: launchd: invalid log app name %q", app)
	}
	dirs, err := xdg.Resolve(getenv)
	if err != nil {
		return "", fmt.Errorf("supervisor: launchd: log dir: %w", err)
	}
	return filepath.Join(dirs.StateHome, app, "logs"), nil
}
