//go:build darwin

package selector

import (
	"context"
	"fmt"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/launchd"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/setsid"
)

// New probes the host and returns launchd when launchctl runs, else
// setsid. Only selection falls back; a chosen backend's later errors
// propagate.
func New(ctx context.Context, exec supervisor.CmdExecutor, o Options) (*Selected, error) {
	if err := checkOptions(exec, o); err != nil {
		return nil, err
	}
	opts := []launchd.Option{launchd.WithGetenv(o.Getenv), launchd.WithMarkers(o.Markers), launchd.WithLogger(o.Logger)}
	if o.LogDir != "" {
		opts = append(opts, launchd.WithLogDir(o.LogDir))
	}
	ld, err := launchd.New(exec, opts...)
	if err != nil {
		return fallback(o, fmt.Sprintf("launchd backend unavailable: %v", err))
	}
	if !ld.Available(ctx) {
		return fallback(o, "launchctl unavailable; session-only durability")
	}
	// A per-user agent starts at login; macOS has no linger equivalent.
	return &Selected{Supervisor: ld, mode: ModeLaunchd, reason: "darwin, launchd agent (starts at login)"}, nil
}

func fallback(o Options, reason string) (*Selected, error) {
	s, err := setsid.New(o.LockDir)
	if err != nil {
		return nil, fmt.Errorf("supervisor: selector: %w", err)
	}
	return &Selected{Supervisor: s, mode: ModeSetsid, reason: reason}, nil
}
