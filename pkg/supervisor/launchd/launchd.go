//go:build darwin

package launchd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/internal/svcenv"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/launchd/internal/logrot"
	"github.com/ludvigalden/toolkit-go/pkg/supervisor/launchd/internal/plist"
	"github.com/ludvigalden/toolkit-go/pkg/xdg"
)

// Defaults for start polling and log rotation.
const (
	defaultStartTimeout = 15 * time.Second
	pollInterval        = 500 * time.Millisecond
	logRotateThreshold  = 10 << 20
	logGenerations      = 3
)

// Launchd is the per-user LaunchAgent backend. Construct it with New.
type Launchd struct {
	exec         supervisor.CmdExecutor
	getenv       func(string) string
	agentsDir    string
	logDir       string
	markers      supervisor.Markers
	logger       *slog.Logger
	startTimeout time.Duration
}

var _ supervisor.Supervisor = (*Launchd)(nil)

// Option configures a Launchd.
type Option func(*Launchd)

// WithGetenv sets the environment reader for HOME and the XDG directories
// (nil: os.Getenv).
func WithGetenv(getenv func(string) string) Option {
	return func(l *Launchd) { l.getenv = getenv }
}

// WithAgentsDir overrides the plist directory (~/Library/LaunchAgents).
func WithAgentsDir(dir string) Option { return func(l *Launchd) { l.agentsDir = dir } }

// WithLogDir sets where per-unit logs go when Spec.StderrLogPath is empty
// (default: DefaultLogDir(spec.Namespace, getenv)).
func WithLogDir(dir string) Option { return func(l *Launchd) { l.logDir = dir } }

// WithMarkers enables maintenance markers: Stop writes one, a successful
// Start clears it.
func WithMarkers(m supervisor.Markers) Option { return func(l *Launchd) { l.markers = m } }

// WithLogger receives best-effort failures (nil: discard).
func WithLogger(lg *slog.Logger) Option { return func(l *Launchd) { l.logger = lg } }

// WithStartTimeout bounds how long Start waits for the agent to run
// (0: 15s).
func WithStartTimeout(d time.Duration) Option { return func(l *Launchd) { l.startTimeout = d } }

// New returns a Launchd running launchctl through exec.
func New(exec supervisor.CmdExecutor, opts ...Option) (*Launchd, error) {
	if exec == nil {
		return nil, errors.New("supervisor: launchd: executor is nil")
	}
	l := &Launchd{exec: exec}
	for _, o := range opts {
		o(l)
	}
	if l.getenv == nil {
		l.getenv = os.Getenv
	}
	if l.logger == nil {
		l.logger = slog.New(slog.DiscardHandler)
	}
	if l.startTimeout <= 0 {
		l.startTimeout = defaultStartTimeout
	}
	if l.agentsDir == "" {
		home := l.getenv("HOME")
		if !filepath.IsAbs(home) {
			return nil, errors.New("supervisor: launchd: $HOME is unset or relative")
		}
		l.agentsDir = filepath.Join(home, "Library", "LaunchAgents")
	}
	return l, nil
}

// Available reports whether launchctl runs. A per-user agent starts at
// login; macOS has no linger equivalent for boot-time start.
func (l *Launchd) Available(ctx context.Context) bool {
	_, _, err := l.exec.Run(ctx, "launchctl", "version")
	return err == nil
}

// Install writes the plist (a byte-identical one is left alone); an empty
// StderrLogPath gets a rotated per-unit log, since launchd has no journal.
// launchd reads a plist only at bootstrap, so a changed plist reloads a
// bootstrapped agent via bootout + bootstrap; a dormant one picks it up
// at the next Start.
func (l *Launchd) Install(ctx context.Context, spec supervisor.Spec) error {
	label, err := Label(spec)
	if err != nil {
		return fmt.Errorf("supervisor: launchd install: %w", err)
	}
	logPath := spec.StderrLogPath
	if logPath == "" {
		if logPath, err = l.unitLog(spec, label); err != nil {
			return fmt.Errorf("supervisor: launchd install: %w", err)
		}
	}
	env, err := svcenv.Capture(l.getenv, spec.ExePath, SystemPATH)
	if err != nil {
		return fmt.Errorf("supervisor: launchd install: %w", err)
	}
	body, err := plist.Render(plist.Agent{
		Label:            label,
		Program:          append([]string{spec.ExePath}, spec.Args...),
		Env:              env,
		WorkingDirectory: spec.WorkingDirectory,
		LogPath:          logPath,
	})
	if err != nil {
		return fmt.Errorf("supervisor: launchd install: %w", err)
	}
	path := filepath.Join(l.agentsDir, label+".plist")
	existing, readErr := os.ReadFile(path)
	if readErr == nil && string(existing) == body {
		return nil
	}
	if err := os.MkdirAll(l.agentsDir, 0o755); err != nil {
		return fmt.Errorf("supervisor: launchd install: %w", err)
	}
	if err := xdg.WriteFileAtomic(l.agentsDir, label+".plist", []byte(body), 0o644); err != nil {
		return fmt.Errorf("supervisor: launchd install: %w", err)
	}
	if errors.Is(readErr, os.ErrNotExist) {
		return nil // fresh install: Start bootstraps it
	}
	uid := l.uid(ctx)
	if _, _, err := l.exec.Run(ctx, "launchctl", "print", target(uid, label)); err != nil {
		return nil //nolint:nilerr // dormant agent: the new plist loads at the next bootstrap
	}
	if _, se, err := l.exec.Run(ctx, "launchctl", "bootout", target(uid, label)); err != nil &&
		!matches(err, se, "not bootstrapped", "Could not find service", "No such process") {
		return fmt.Errorf("supervisor: launchd install: bootout: %w", err)
	}
	if _, _, err := l.exec.Run(ctx, "launchctl", "bootstrap", "gui/"+strconv.Itoa(uid), path); err != nil {
		return fmt.Errorf("supervisor: launchd install: bootstrap: %w", err)
	}
	return nil
}

func (l *Launchd) unitLog(spec supervisor.Spec, label string) (string, error) {
	dir := l.logDir
	if dir == "" {
		var err error
		if dir, err = DefaultLogDir(spec.Namespace, l.getenv); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("supervisor: launchd log dir: %w", err)
	}
	live := filepath.Join(dir, label+".log")
	if err := logrot.Rotate(live, logRotateThreshold, logGenerations); err != nil {
		l.logger.Warn("supervisor: launchd log rotation skipped", "path", live, "error", err)
	}
	return live, nil
}

// Start bootstraps the agent into gui/<uid> and polls until it runs. An
// already-bootstrapped agent is not an error.
func (l *Launchd) Start(ctx context.Context, spec supervisor.Spec) error {
	label, err := Label(spec)
	if err != nil {
		return fmt.Errorf("supervisor: launchd start: %w", err)
	}
	uid := l.uid(ctx)
	path := filepath.Join(l.agentsDir, label+".plist")
	if _, se, err := l.exec.Run(ctx, "launchctl", "bootstrap", "gui/"+strconv.Itoa(uid), path); err != nil &&
		!matches(err, se, "already bootstrapped") {
		return fmt.Errorf("supervisor: launchd start: bootstrap: %w", err)
	}
	target := target(uid, label)
	deadline := time.Now().Add(l.startTimeout)
	last := "unknown"
	for {
		if out, _, err := l.exec.Run(ctx, "launchctl", "print", target); err == nil {
			if last = plist.State(string(out)); last == "running" {
				if err := l.markers.Clear(label); err != nil {
					l.logger.Debug("supervisor: clear maintenance marker", "label", label, "error", err)
				}
				return nil
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("supervisor: launchd start: agent %q not running within %s (state %q)", label, l.startTimeout, last)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("supervisor: launchd start: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// Stop records the maintenance marker (reason from ctx, default stop) and
// boots the agent out of the session; the plist stays on disk.
func (l *Launchd) Stop(ctx context.Context, spec supervisor.Spec) error {
	label, err := Label(spec)
	if err != nil {
		return fmt.Errorf("supervisor: launchd stop: %w", err)
	}
	tool, err := spec.Tool()
	if err != nil {
		return fmt.Errorf("supervisor: launchd stop: %w", err)
	}
	reason := supervisor.MaintenanceReasonFromContext(ctx)
	if reason == "" {
		reason = supervisor.MaintenanceReasonStop
	}
	if err := l.markers.Write(label, tool, reason, time.Now()); err != nil {
		l.logger.Debug("supervisor: write maintenance marker", "label", label, "error", err)
	}
	return l.bootout(ctx, "stop", label)
}

// Status reports the agent's state from launchctl print; Installed
// reflects the plist on disk.
func (l *Launchd) Status(ctx context.Context, spec supervisor.Spec) (supervisor.Status, error) {
	label, err := Label(spec)
	if err != nil {
		return supervisor.Status{}, fmt.Errorf("supervisor: launchd status: %w", err)
	}
	_, statErr := os.Stat(filepath.Join(l.agentsDir, label+".plist"))
	st := supervisor.Status{Mode: Mode, Installed: statErr == nil}
	out, _, err := l.exec.Run(ctx, "launchctl", "print", target(l.uid(ctx), label))
	if err != nil {
		return st, nil //nolint:nilerr // an unloaded agent is a negative probe, not a failure
	}
	state := plist.State(string(out))
	st.Running = state == "running"
	if st.Running {
		st.PID = plist.PID(string(out))
	} else if state != "" {
		st.Diagnostic = "state=" + state
	}
	return st, nil
}

// Uninstall boots the agent out and removes its plist.
func (l *Launchd) Uninstall(ctx context.Context, spec supervisor.Spec) error {
	label, err := Label(spec)
	if err != nil {
		return fmt.Errorf("supervisor: launchd uninstall: %w", err)
	}
	if err := l.bootout(ctx, "uninstall", label); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(l.agentsDir, label+".plist")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("supervisor: launchd uninstall: %w", err)
	}
	return nil
}

func (l *Launchd) bootout(ctx context.Context, op, label string) error {
	_, se, err := l.exec.Run(ctx, "launchctl", "bootout", target(l.uid(ctx), label))
	if err != nil && !matches(err, se, "not bootstrapped", "Could not find service", "No such process") {
		return fmt.Errorf("supervisor: launchd %s: bootout: %w", op, err)
	}
	return nil
}

// uid asks the executor so tests control the gui domain; os.Getuid is the
// fallback.
func (l *Launchd) uid(ctx context.Context) int {
	if out, _, err := l.exec.Run(ctx, "id", "-u"); err == nil {
		if uid, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			return uid
		}
	}
	return os.Getuid()
}

func target(uid int, label string) string { return "gui/" + strconv.Itoa(uid) + "/" + label }

// matches checks launchctl's message text, the only stable surface once
// the executor has wrapped the exit error.
func matches(err error, stderr []byte, needles ...string) bool {
	msg := err.Error() + "\n" + string(stderr)
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}
