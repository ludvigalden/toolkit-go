package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaintenanceMarkerTTL bounds how long a marker marks a stop as planned.
const MaintenanceMarkerTTL = 10 * time.Minute

// MaintenanceReason names why a tooling-initiated stop is happening.
type MaintenanceReason string

// Maintenance reasons; undecorated stops use MaintenanceReasonStop.
const (
	MaintenanceReasonRestart MaintenanceReason = "restart"
	MaintenanceReasonRecover MaintenanceReason = "recover"
	MaintenanceReasonStop    MaintenanceReason = "stop"
)

type maintenanceReasonKey struct{}

// WithMaintenanceReason decorates ctx so a backend's Stop records reason
// in the maintenance marker.
func WithMaintenanceReason(ctx context.Context, reason MaintenanceReason) context.Context {
	return context.WithValue(ctx, maintenanceReasonKey{}, reason)
}

// MaintenanceReasonFromContext returns the decorated reason, or "".
func MaintenanceReasonFromContext(ctx context.Context) MaintenanceReason {
	r, _ := ctx.Value(maintenanceReasonKey{}).(MaintenanceReason)
	return r
}

// Marker is a parsed maintenance marker.
type Marker struct {
	Expires time.Time `json:"-"`
	Unit    string    `json:"unit"`
	Tool    string    `json:"tool"`
	Reason  string    `json:"reason"`
}

// Active reports whether the marker has not yet expired at now.
func (m Marker) Active(now time.Time) bool { return now.Before(m.Expires) }

// Markers stores maintenance markers in Dir, so failure alerting can tell
// a planned stop from a crash. The body is an RFC 3339 expiry line, then a
// JSON line. An empty Dir disables markers: every method is a no-op.
type Markers struct {
	Dir string
}

// RuntimeMarkers returns Markers under $XDG_RUNTIME_DIR/<app>/maintenance,
// read through getenv (nil: os.Getenv). Without a runtime dir markers are
// disabled, since a stop must never fail for want of one. An invalid app
// name is an error, never a silent disable.
func RuntimeMarkers(app string, getenv func(string) string) (Markers, error) {
	if err := checkSegment("marker app", app); err != nil {
		return Markers{}, err
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	rd := getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(rd) {
		return Markers{}, nil
	}
	return Markers{Dir: filepath.Join(rd, app, "maintenance")}, nil
}

// Write records a marker for unit that expires after MaintenanceMarkerTTL
// and sweeps expired siblings.
func (m Markers) Write(unit, tool string, reason MaintenanceReason, now time.Time) error {
	if m.Dir == "" {
		return nil
	}
	if err := checkSegment("marker unit", unit); err != nil {
		return err
	}
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return fmt.Errorf("supervisor: maintenance marker dir: %w", err)
	}
	line2, err := json.Marshal(Marker{Unit: unit, Tool: tool, Reason: string(reason)})
	if err != nil {
		return fmt.Errorf("supervisor: maintenance marker encode: %w", err)
	}
	body := now.UTC().Add(MaintenanceMarkerTTL).Format(time.RFC3339Nano) + "\n" + string(line2) + "\n"
	if err := os.WriteFile(m.path(unit), []byte(body), 0o600); err != nil {
		return fmt.Errorf("supervisor: maintenance marker write: %w", err)
	}
	m.sweep(now)
	return nil
}

// Read returns unit's marker; found is false when none exists.
func (m Markers) Read(unit string) (marker Marker, found bool, err error) {
	if m.Dir == "" {
		return Marker{}, false, nil
	}
	body, err := os.ReadFile(m.path(unit))
	if errors.Is(err, os.ErrNotExist) {
		return Marker{}, false, nil
	}
	if err != nil {
		return Marker{}, false, fmt.Errorf("supervisor: maintenance marker read: %w", err)
	}
	marker, err = parseMarker(body)
	if err != nil {
		return Marker{}, true, err
	}
	return marker, true, nil
}

// Clear removes unit's marker; an absent marker is not an error.
func (m Markers) Clear(unit string) error {
	if m.Dir == "" {
		return nil
	}
	if err := os.Remove(m.path(unit)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("supervisor: maintenance marker clear: %w", err)
	}
	return nil
}

func (m Markers) path(unit string) string { return filepath.Join(m.Dir, unit+".marker") }

// sweep removes expired markers; malformed ones stay for readers to judge.
func (m Markers) sweep(now time.Time) {
	entries, err := os.ReadDir(m.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".marker") {
			continue
		}
		path := filepath.Join(m.Dir, e.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if mk, err := parseMarker(body); err == nil && !mk.Active(now) {
			_ = os.Remove(path) //nolint:errcheck // best-effort sweep
		}
	}
}

func parseMarker(body []byte) (Marker, error) {
	line1, line2, _ := strings.Cut(string(body), "\n")
	expires, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(line1))
	if err != nil {
		return Marker{}, fmt.Errorf("supervisor: maintenance marker expiry: %w", err)
	}
	var mk Marker
	if err := json.Unmarshal([]byte(strings.TrimSpace(line2)), &mk); err != nil {
		return Marker{}, fmt.Errorf("supervisor: maintenance marker body: %w", err)
	}
	mk.Expires = expires
	return mk, nil
}
