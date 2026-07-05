//go:build linux

// Package systemd is the Linux systemd-user supervisor backend. Every
// systemctl and loginctl call goes through a supervisor.CmdExecutor.
package systemd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/internal/svcenv"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/systemd/internal/unit"
	"github.com/ludvigalden/toolkit-go/pkg/xdg"
)

// Mode is the Status.Mode this backend reports.
const Mode = "systemd"

// SystemPATH follows the executable's directory in every unit's PATH.
const SystemPATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// Start polling defaults.
const (
	defaultStartTimeout = 15 * time.Second
	defaultPollInterval = 500 * time.Millisecond
)

// Systemd is the systemd-user backend. Construct it with New.
type Systemd struct {
	exec         supervisor.CmdExecutor
	getenv       func(string) string
	unitDir      string
	markers      supervisor.Markers
	logger       *slog.Logger
	startTimeout time.Duration
	pollInterval time.Duration
}

var _ supervisor.Supervisor = (*Systemd)(nil)

// Option configures a Systemd.
type Option func(*Systemd)

// WithGetenv sets the environment reader used for HOME, the XDG
// directories and USER (nil: os.Getenv).
func WithGetenv(getenv func(string) string) Option {
	return func(s *Systemd) { s.getenv = getenv }
}

// WithUnitDir overrides the unit directory ($XDG_CONFIG_HOME/systemd/user).
func WithUnitDir(dir string) Option { return func(s *Systemd) { s.unitDir = dir } }

// WithMarkers enables maintenance markers: Stop writes one, a successful
// Start clears it.
func WithMarkers(m supervisor.Markers) Option { return func(s *Systemd) { s.markers = m } }

// WithLogger receives best-effort failures (nil: discard).
func WithLogger(l *slog.Logger) Option { return func(s *Systemd) { s.logger = l } }

// WithStartTimeout bounds how long Start waits for the unit to become
// active (0: 15s).
func WithStartTimeout(d time.Duration) Option { return func(s *Systemd) { s.startTimeout = d } }

// New returns a Systemd running commands through exec.
func New(exec supervisor.CmdExecutor, opts ...Option) (*Systemd, error) {
	if exec == nil {
		return nil, errors.New("supervisor: systemd: executor is nil")
	}
	s := &Systemd{exec: exec, startTimeout: defaultStartTimeout, pollInterval: defaultPollInterval}
	for _, o := range opts {
		o(s)
	}
	if s.getenv == nil {
		s.getenv = os.Getenv
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	if s.startTimeout <= 0 {
		s.startTimeout = defaultStartTimeout
	}
	if s.unitDir == "" {
		dirs, err := xdg.Resolve(s.getenv)
		if err != nil {
			return nil, fmt.Errorf("supervisor: systemd: unit dir: %w", err)
		}
		s.unitDir = filepath.Join(dirs.ConfigHome, "systemd", "user")
	}
	return s, nil
}

// UnitFileName returns spec's unit file name, <identity>.service.
func UnitFileName(spec supervisor.Spec) (string, error) {
	id, err := spec.Identity()
	if err != nil {
		return "", err //nolint:wrapcheck // carries the supervisor prefix
	}
	return id + ".service", nil
}

// Available reports whether systemd-user can supervise boot-durable
// services: linger is enabled and the user manager is running or
// degraded. It never enables linger itself.
func (s *Systemd) Available(ctx context.Context) bool {
	name := s.username()
	if name == "" {
		return false
	}
	out, _, err := s.exec.Run(ctx, "loginctl", "show-user", name, "--property=Linger")
	if err != nil || !strings.Contains(string(out), "Linger=yes") {
		return false
	}
	// is-system-running exits 1 for degraded, so the state word decides.
	out, _, _ = s.exec.Run(ctx, "systemctl", "--user", "is-system-running") //nolint:errcheck // see above
	switch strings.TrimSpace(string(out)) {
	case "running", "degraded":
		return true
	}
	return false
}

func (s *Systemd) username() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return s.getenv("USER")
}

// Install writes the unit file and reloads systemd. A byte-identical
// existing unit is left alone, so repeated installs are cheap while
// template changes still propagate.
func (s *Systemd) Install(ctx context.Context, spec supervisor.Spec) error {
	name, err := UnitFileName(spec)
	if err != nil {
		return fmt.Errorf("supervisor: systemd install: %w", err)
	}
	env, err := svcenv.Capture(s.getenv, spec.ExePath, SystemPATH)
	if err != nil {
		return fmt.Errorf("supervisor: systemd install: %w", err)
	}
	body, err := unit.Render(spec, env)
	if err != nil {
		return fmt.Errorf("supervisor: systemd install: %w", err)
	}
	if err := os.MkdirAll(s.unitDir, 0o755); err != nil {
		return fmt.Errorf("supervisor: systemd install: %w", err)
	}
	if existing, err := os.ReadFile(filepath.Join(s.unitDir, name)); err == nil && string(existing) == body {
		return nil
	}
	if err := xdg.WriteFileAtomic(s.unitDir, name, []byte(body), 0o644); err != nil {
		return fmt.Errorf("supervisor: systemd install: %w", err)
	}
	return s.systemctl(ctx, "install", "daemon-reload")
}

// Start resets a failed unit, starts it and polls until it is active. It
// fails fast on a terminal state instead of polling out a dead unit.
func (s *Systemd) Start(ctx context.Context, spec supervisor.Spec) error {
	base, tool, err := names(spec)
	if err != nil {
		return fmt.Errorf("supervisor: systemd start: %w", err)
	}
	s.resetFailedIfNeeded(ctx, base)
	if err := s.systemctl(ctx, "start", "start", base); err != nil {
		return err
	}
	deadline := time.Now().Add(s.startTimeout)
	last := "unknown"
	for {
		out, _, err := s.exec.Run(ctx, "systemctl", "--user", "show", base,
			"--property=ActiveState,SubState,Result,ExecMainStatus")
		if err == nil {
			p := unit.ParseShow(string(out))
			last = fmt.Sprintf("ActiveState=%s SubState=%s Result=%s ExecMainStatus=%s",
				p["ActiveState"], p["SubState"], p["Result"], p["ExecMainStatus"])
			if p["ActiveState"] == "active" {
				if err := s.markers.Clear(base + ".service"); err != nil {
					s.logger.Debug("supervisor: clear maintenance marker", "unit", base, "tool", tool, "error", err)
				}
				return nil
			}
			if f := unit.StartFailure(p); f != "" {
				return fmt.Errorf("supervisor: systemd start: unit %q %s; see journalctl --user -u %s", base, f, base)
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("supervisor: systemd start: unit %q not active within %s (%s)", base, s.startTimeout, last)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("supervisor: systemd start: %w", ctx.Err())
		case <-time.After(s.pollInterval):
		}
	}
}

// resetFailedIfNeeded clears a failed or restart-cycling state that would
// make systemd refuse the start.
func (s *Systemd) resetFailedIfNeeded(ctx context.Context, base string) {
	out, _, err := s.exec.Run(ctx, "systemctl", "--user", "show", base, "--property=ActiveState,SubState", "--value")
	if err != nil {
		return
	}
	f := strings.Fields(string(out))
	if len(f) == 0 || f[0] != "failed" && (len(f) < 2 || f[1] != "restart" && f[1] != "auto-restart") {
		return
	}
	if _, _, err := s.exec.Run(ctx, "systemctl", "--user", "reset-failed", base); err != nil {
		s.logger.Debug("supervisor: systemd reset-failed", "unit", base, "error", err)
	}
}

// Stop writes the maintenance marker (reason from ctx, default stop) and
// stops the unit; systemd owns the SIGTERM to SIGKILL escalation.
func (s *Systemd) Stop(ctx context.Context, spec supervisor.Spec) error {
	base, tool, err := names(spec)
	if err != nil {
		return fmt.Errorf("supervisor: systemd stop: %w", err)
	}
	reason := supervisor.MaintenanceReasonFromContext(ctx)
	if reason == "" {
		reason = supervisor.MaintenanceReasonStop
	}
	if err := s.markers.Write(base+".service", tool, reason, time.Now()); err != nil {
		s.logger.Debug("supervisor: write maintenance marker", "unit", base, "error", err)
	}
	return s.systemctl(ctx, "stop", "stop", base)
}

// Status reports the unit's state from systemctl show; Installed reflects
// the unit file on disk.
func (s *Systemd) Status(ctx context.Context, spec supervisor.Spec) (supervisor.Status, error) {
	base, _, err := names(spec)
	if err != nil {
		return supervisor.Status{}, fmt.Errorf("supervisor: systemd status: %w", err)
	}
	_, statErr := os.Stat(filepath.Join(s.unitDir, base+".service"))
	st := supervisor.Status{Mode: Mode, Installed: statErr == nil}
	out, _, err := s.exec.Run(ctx, "systemctl", "--user", "show", base,
		"--property=ActiveState,SubState,ExecMainPID,ExecMainStatus,Result")
	if err != nil {
		return st, nil //nolint:nilerr // an unloaded unit is a negative probe, not a failure
	}
	p := unit.ParseShow(string(out))
	active, sub := p["ActiveState"], p["SubState"]
	st.Running = active == "active" && sub == "running"
	if st.Running {
		st.PID, _ = strconv.Atoi(p["ExecMainPID"]) //nolint:errcheck // 0 on garbage
	}
	if ems, ok := p["ExecMainStatus"]; ok && ems != "-1" {
		st.LastExitStatus = ems
	}
	if active != "" && !st.Running {
		st.Diagnostic = fmt.Sprintf("ActiveState=%s SubState=%s", active, sub)
		// Result survives until reset-failed, so it tells a rate-limited
		// unit from a merely stopped one.
		if r := p["Result"]; r != "" && r != "success" {
			st.Diagnostic += " Result=" + r
		}
	}
	return st, nil
}

// Uninstall disables the unit, removes its file and reloads systemd.
func (s *Systemd) Uninstall(ctx context.Context, spec supervisor.Spec) error {
	base, _, err := names(spec)
	if err != nil {
		return fmt.Errorf("supervisor: systemd uninstall: %w", err)
	}
	if _, se, err := s.exec.Run(ctx, "systemctl", "--user", "disable", base); err != nil && !notLoaded(err, se) {
		return fmt.Errorf("supervisor: systemd uninstall: disable: %w", err)
	}
	if err := os.Remove(filepath.Join(s.unitDir, base+".service")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("supervisor: systemd uninstall: %w", err)
	}
	return s.systemctl(ctx, "uninstall", "daemon-reload")
}

func (s *Systemd) systemctl(ctx context.Context, op string, args ...string) error {
	if _, _, err := s.exec.Run(ctx, "systemctl", append([]string{"--user"}, args...)...); err != nil {
		return fmt.Errorf("supervisor: systemd %s: %w", op, err)
	}
	return nil
}

// names returns the bare unit name systemctl expects and the tool segment.
func names(spec supervisor.Spec) (base, tool string, err error) {
	base, err = spec.Identity()
	if err != nil {
		return "", "", err //nolint:wrapcheck // carries the supervisor prefix
	}
	tool, err = spec.Tool()
	return base, tool, err //nolint:wrapcheck // carries the supervisor prefix
}

// notLoaded matches systemctl's unloaded-unit message; the text is the
// only stable surface once the executor has wrapped the exit error.
func notLoaded(err error, stderr []byte) bool {
	msg := err.Error() + "\n" + string(stderr)
	return strings.Contains(msg, "not loaded") || strings.Contains(msg, "No such file or directory") ||
		strings.Contains(msg, "Unit") && strings.Contains(msg, "not found")
}
