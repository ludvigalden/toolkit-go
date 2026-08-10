package scratch

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/scratch/internal/fencing"
	"github.com/ludvigalden/toolkit-go/pkg/scratch/internal/pathsafe"
)

// Path-safety sentinels, re-exported so callers can classify failures
// with errors.Is without importing the internal mechanics package.
var (
	// ErrTraversal reports a root-relative path that would leave the
	// scratch root: an absolute path, a ".." element, or the root
	// itself.
	ErrTraversal = pathsafe.ErrTraversal

	// ErrSymlinkEscape reports a symlinked path component inside the
	// scratch root through which a write could leave the root.
	ErrSymlinkEscape = pathsafe.ErrSymlinkEscape
)

// Env is the concrete lifecycle owner of one scratch environment. It
// exposes the scratch root and the fenced child environment, performs
// path-safe writes and seeding inside the root, and owns teardown.
type Env struct {
	root            string
	workDir         string
	childEnv        []string
	stderr          io.Writer
	retainOnFailure bool
	failed          bool
	retained        bool
}

// Option configures New.
type Option func(*fencing.Config)

// WithRootParent sets the directory the scratch root is created in.
// Defaults to the system temporary directory.
func WithRootParent(dir string) Option {
	return func(c *fencing.Config) { c.Parent = dir }
}

// WithPrefix sets the scratch root name prefix. Defaults to "scratch-".
func WithPrefix(prefix string) Option {
	return func(c *fencing.Config) { c.Prefix = prefix }
}

// WithWorkDir sets the working directory as a root-relative path.
// Defaults to "work".
func WithWorkDir(rel string) Option {
	return func(c *fencing.Config) { c.WorkRel = rel }
}

// WithFencedVar declares an additional tool-specific fenced variable:
// key is set to the root-joined rel path inside the root, and the
// directory is created during setup. The six core variables cannot be
// redefined.
func WithFencedVar(key, rel string) Option {
	return func(c *fencing.Config) {
		c.Extras = append(c.Extras, fencing.FencedVar{Key: key, Rel: rel})
	}
}

// WithStderr sets the writer the retained-path notice is printed on.
// Defaults to a sink that emits one slog.Warn record per notice, so
// the notice follows the process-wide log configuration.
func WithStderr(w io.Writer) Option {
	return func(c *fencing.Config) { c.Stderr = w }
}

// WithRetainOnFailure keeps the scratch root when the run is declared
// failed or setup fails, printing the retained path on stderr.
func WithRetainOnFailure() Option {
	return func(c *fencing.Config) { c.RetainOnFailure = true }
}

// diagWriter is the default notice sink: each Write emits one slog.Warn
// record with the trailing newline stripped.
type diagWriter struct{}

func (diagWriter) Write(p []byte) (int, error) {
	slog.Warn(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// New builds a scratch environment and returns its concrete lifecycle
// owner. Setup failures remove partial state by default; under
// WithRetainOnFailure the partial root is retained and its path printed
// through the notice sink before the error is returned.
func New(opts ...Option) (*Env, error) {
	cfg := fencing.Config{
		Prefix:  fencing.DefaultPrefix,
		WorkRel: fencing.DefaultWorkRel,
		Stderr:  diagWriter{},
		Environ: os.Environ,
		TempDir: os.TempDir,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	st, err := fencing.Create(cfg)
	if err != nil {
		return nil, err
	}
	return &Env{
		root:            st.Root,
		workDir:         st.WorkDir,
		childEnv:        st.ChildEnv,
		stderr:          cfg.Stderr,
		retainOnFailure: cfg.RetainOnFailure,
	}, nil
}

// Root returns the absolute scratch root path.
func (e *Env) Root() string { return e.root }

// Dir returns the absolute working directory inside the root.
func (e *Env) Dir() string { return e.workDir }

// ChildEnv returns the fenced child environment: a snapshot of the
// constructing process environment with the six core variables and any
// declared extras redirected inside the root. Treat the result as
// read-only.
func (e *Env) ChildEnv() []string { return e.childEnv }

// Fail declares the run failed. With WithRetainOnFailure set, Cleanup
// then keeps the root and prints its path on stderr.
func (e *Env) Fail() { e.failed = true }

// Cleanup tears the scratch root down. The default policy removes it —
// also after a declared failure. With WithRetainOnFailure and a
// declared failure, the root is kept and the retained path is printed
// on stderr once. Cleanup is idempotent.
func (e *Env) Cleanup() error {
	if e.failed && e.retainOnFailure {
		if !e.retained {
			e.retained = true
			_, _ = fmt.Fprintf(e.stderr,
				"scratch: retained for failure inspection: %s\n", e.root)
		}
		return nil
	}
	return fencing.Remove(e.root)
}
