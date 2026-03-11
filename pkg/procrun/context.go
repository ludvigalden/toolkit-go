package procrun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/lookup"
	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/shell"
)

// DiagKind classifies a subprocess lifecycle event for observers.
type DiagKind string

const (
	// DiagLaunch reports a spawned subprocess start.
	DiagLaunch DiagKind = "launch"
	// DiagShellSpawn reports procrun spawned the user's shell.
	DiagShellSpawn DiagKind = "shell_spawn"
	// DiagShellKill reports a spawned shell's process group was killed.
	DiagShellKill DiagKind = "shell_kill"
	// DiagGroupKill reports a direct-exec process group was killed.
	DiagGroupKill DiagKind = "group_kill"
	// DiagTimeout reports a subprocess timeout.
	DiagTimeout DiagKind = "timeout"
	// DiagExit reports a finished Run; Elapsed lets observers total
	// subprocess wall time per context.
	DiagExit DiagKind = "exit"
)

// DiagEvent is one subprocess lifecycle event; Detail carries
// kind-specific context.
type DiagEvent struct {
	Kind    DiagKind
	Name    string
	Detail  string
	Elapsed time.Duration
}

// Resolver maps a tool name to an absolute executable path. It must be
// side-effect free (no spawns, no network): resolvers run before PATH
// lookup and deliberately shadow same-named PATH binaries.
type Resolver interface {
	Resolve(name string) (path string, ok bool)
}

// Scoper places a subprocess under resource governance. Wrap may rewrite
// cmd before it starts and returns the release run after exit; a nil
// release means nothing to undo. Only Specs naming a Profile are wrapped.
type Scoper interface {
	Wrap(ctx context.Context, profile string, cmd *exec.Cmd) (release func(context.Context) error, err error)
}

// ScoperFunc adapts a function to Scoper.
type ScoperFunc func(ctx context.Context, profile string, cmd *exec.Cmd) (release func(context.Context) error, err error)

// Wrap calls f.
func (f ScoperFunc) Wrap(ctx context.Context, profile string, cmd *exec.Cmd) (func(context.Context) error, error) {
	return f(ctx, profile, cmd)
}

// RunContextOption configures a RunContext at construction.
type RunContextOption func(*RunContext)

// WithEnv appends extraEnv to the process environment for every spawn;
// repeated calls (including through Derive) accumulate.
func WithEnv(extraEnv ...string) RunContextOption {
	return func(rc *RunContext) {
		rc.extraEnv = append(slices.Clip(rc.extraEnv), extraEnv...)
	}
}

// WithEnvSource injects the process environment: getenv answers named
// lookups ($SHELL) and environ enumerates it for spawns (each nil value
// falls back to os.Getenv / os.Environ).
func WithEnvSource(getenv func(string) string, environ func() []string) RunContextOption {
	return func(rc *RunContext) { rc.getenv, rc.environ = getenv, environ }
}

// WithResolver appends r to the resolver chain (run in registration order).
func WithResolver(r Resolver) RunContextOption {
	return func(rc *RunContext) { rc.resolvers = append(rc.resolvers, r) }
}

// WithRegistry tracks every process spawned through the context in reg.
func WithRegistry(reg *Registry) RunContextOption {
	return func(rc *RunContext) { rc.registry = reg }
}

// WithScoper attaches a Scoper for Specs that name a Profile.
func WithScoper(sc Scoper) RunContextOption {
	return func(rc *RunContext) { rc.scoper = sc }
}

// WithObserver receives lifecycle events for the context's spawns. It runs
// synchronously on spawn paths and must not block.
func WithObserver(fn func(DiagEvent)) RunContextOption {
	return func(rc *RunContext) { rc.observer = fn }
}

// WithShellChildEnv names the variable exported as "=1" into every shell
// procrun spawns, so shell init can detect the nested context and avoid
// recursion. The default is DefaultShellChildEnv.
func WithShellChildEnv(name string) RunContextOption {
	return func(rc *RunContext) {
		if name != "" {
			rc.shellChildEnv = name
		}
	}
}

// DefaultShellChildEnv is the shell-child marker exported when no
// WithShellChildEnv is configured; shell init scripts test it to skip
// eager work inside procrun-spawned shells.
const DefaultShellChildEnv = "PROCRUN_SHELL_CHILD"

// RunContext carries the settings shared by a family of subprocess
// invocations. Options are fixed at construction; use Derive to add
// per-launch options. A nil *RunContext behaves as the default context.
type RunContext struct {
	dir           string
	extraEnv      []string
	resolvers     []Resolver
	registry      *Registry
	scoper        Scoper
	observer      func(DiagEvent)
	shellChildEnv string
	paths         *lookup.Cache
	getenv        func(string) string
	environ       func() []string
}

// NewRunContext creates a RunContext rooted at dir ("" inherits the
// current directory).
func NewRunContext(dir string, opts ...RunContextOption) *RunContext {
	rc := &RunContext{dir: dir, shellChildEnv: DefaultShellChildEnv, paths: &lookup.Cache{}}
	for _, opt := range opts {
		opt(rc)
	}
	return rc
}

// Derive returns a copy of c with opts applied; c is never mutated. The
// copy shares c's resolution cache.
func (c *RunContext) Derive(opts ...RunContextOption) *RunContext {
	c = c.orDefault()
	child := *c
	child.resolvers = append([]Resolver(nil), c.resolvers...)
	for _, opt := range opts {
		opt(&child)
	}
	return &child
}

// Cmd creates a Spec resolved through this context on Run or Start.
func (c *RunContext) Cmd(name string, args ...string) *Spec {
	c = c.orDefault()
	return &Spec{name: name, args: args, rc: c, dir: c.dir, env: c.spawnEnv()}
}

// CmdPath creates a Spec for an already-resolved binary path.
func (c *RunContext) CmdPath(path string, args ...string) *Spec {
	s := c.Cmd(path, args...)
	s.path = path
	return s
}

// Find resolves name to an absolute executable: an absolute executable
// name as-is, then the resolver chain, then PATH, then one
// non-interactive shell probe ($SHELL -c "command -v name").
func (c *RunContext) Find(name string) (string, error) {
	c = c.orDefault()
	if p, ok := c.resolve(name); ok {
		return p, nil
	}
	if p, ok := c.paths.Get("shell:" + name); ok {
		return p, nil
	}
	shell := c.envLookup("SHELL")
	if shell != "" {
		if p, err := c.probeShell(shell, name); err == nil {
			c.paths.Put("shell:"+name, p)
			return p, nil
		}
	}
	return "", &ToolNotFoundError{Name: name}
}

// FindWithoutShell is Find without the shell probe.
func (c *RunContext) FindWithoutShell(name string) (string, error) {
	if p, ok := c.orDefault().resolve(name); ok {
		return p, nil
	}
	return "", &ToolNotFoundError{Name: name}
}

// resolve runs the absolute-path, resolver-chain, and PATH stages. Only
// hits are cached so a tool installed mid-run stays discoverable.
func (c *RunContext) resolve(name string) (string, bool) {
	if filepath.IsAbs(name) && lookup.IsExecutable(name) {
		return name, true
	}
	for _, r := range c.resolvers {
		if p, ok := r.Resolve(name); ok && filepath.IsAbs(p) {
			return p, true
		}
	}
	return lookup.Path(c.paths, name)
}

// probeShell runs `shell -c "command -v name"` in its own process group.
func (c *RunContext) probeShell(sh, name string) (string, error) {
	c.emit(DiagShellSpawn, name, "probe: "+sh+" -c")
	env := append(c.spawnEnvOrProcess(), c.shellChildEnv+"=1")
	return shell.Probe(sh, name, env, func(cmd *exec.Cmd) { //nolint:wrapcheck // Find maps failure to ToolNotFoundError
		c.applyGroupKill(cmd, name, DiagShellKill, 0)
	})
}

// envLookup answers a named lookup through the injected environment
// source, falling back to the process environment.
func (c *RunContext) envLookup(key string) string {
	if c.getenv != nil {
		return c.getenv(key)
	}
	return os.Getenv(key)
}

// spawnEnv is the environment a spawn receives; nil inherits the process
// environment, which is only correct when nothing is injected or added.
func (c *RunContext) spawnEnv() []string {
	if c.environ == nil && len(c.extraEnv) == 0 {
		return nil
	}
	return append(slices.Clip(c.envList()), c.extraEnv...)
}

// spawnEnvOrProcess is spawnEnv with the inherited case made explicit, for
// callers that must append to it.
func (c *RunContext) spawnEnvOrProcess() []string {
	if env := c.spawnEnv(); env != nil {
		return env
	}
	return os.Environ()
}

// envList enumerates the environment through the injected source,
// falling back to the process environment.
func (c *RunContext) envList() []string {
	if c.environ != nil {
		return c.environ()
	}
	return os.Environ()
}

// emit reports one lifecycle event to the observer, if any.
func (c *RunContext) emit(kind DiagKind, name, detail string) {
	if c.observer != nil {
		c.observer(DiagEvent{Kind: kind, Name: name, Detail: detail})
	}
}

// orDefault maps a nil receiver to the package default context.
func (c *RunContext) orDefault() *RunContext {
	if c == nil {
		return defaultContext()
	}
	if c.shellChildEnv == "" {
		// A zero-value RunContext literal: fill the marker on a copy.
		cp := *c
		cp.shellChildEnv = DefaultShellChildEnv
		return &cp
	}
	return c
}

// defaultContext backs the package-level helpers: no registry, resolvers,
// scoper, or observer, and no resolution cache, so PATH changes are seen.
func defaultContext() *RunContext {
	return &RunContext{shellChildEnv: DefaultShellChildEnv}
}

// Cmd creates a Spec on the default context, resolved via PATH (and no
// shell probe unless ShellFallback is set).
func Cmd(name string, args ...string) *Spec { return defaultContext().Cmd(name, args...) }

// CmdPath creates a Spec on the default context for an explicit path.
func CmdPath(path string, args ...string) *Spec { return defaultContext().CmdPath(path, args...) }

// Find resolves name on PATH (absolute executables pass through), uncached.
// It never spawns a shell; use a RunContext's Find for the shell probe.
func Find(name string) (string, error) { return defaultContext().FindWithoutShell(name) }
