//go:build linux

package selector

import (
	"context"
	"fmt"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/setsid"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/systemd"
)

// New probes the host and returns systemd-user when linger is enabled and
// the user manager runs, else setsid. Only selection falls back; a chosen
// backend's later errors propagate.
func New(ctx context.Context, exec supervisor.CmdExecutor, o Options) (*Selected, error) {
	if err := checkOptions(exec, o); err != nil {
		return nil, err
	}
	sd, err := systemd.New(exec, systemd.WithGetenv(o.Getenv), systemd.WithMarkers(o.Markers), systemd.WithLogger(o.Logger))
	if err != nil {
		return fallback(o, fmt.Sprintf("systemd backend unavailable: %v", err))
	}
	if !sd.Available(ctx) {
		return fallback(o, "systemd-user unavailable (linger disabled or user manager offline); session-only durability")
	}
	return &Selected{Supervisor: sd, mode: ModeSystemd, reason: "linux, systemd-user with linger (boot-durable)"}, nil
}

func fallback(o Options, reason string) (*Selected, error) {
	s, err := setsid.New(o.LockDir)
	if err != nil {
		return nil, fmt.Errorf("supervisor: selector: %w", err)
	}
	return &Selected{Supervisor: s, mode: ModeSetsid, reason: reason}, nil
}
