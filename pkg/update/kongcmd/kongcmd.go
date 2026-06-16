// Package kongcmd provides the version, changelog and update commands
// as Kong command structs. A tool embeds the ones it wants and binds a
// *Deps; data goes to stdout, diagnostics to stderr, and --json selects
// a machine-readable payload.
package kongcmd

import (
	"context"
	"io"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/kongcmd/internal/flow"
	"github.com/ludvigalden/toolkit-go/pkg/update/relmapx"
)

// Deps carries what the commands need. Any capability may be nil; the
// command that needs it then reports it unavailable.
type Deps struct {
	Tool    string
	Version string
	// Build is the embedded relmap build state; optional.
	Build *relmapx.BuildState
	// Changelog presents release notes; optional.
	Changelog *update.Changelog
	// Checker finds the latest release; optional.
	Checker update.Checker
	// Updater replaces the binary; optional.
	Updater update.Updater
	// Context defaults to context.Background.
	Context context.Context
	Stdout  io.Writer
	Stderr  io.Writer
}

func (d *Deps) io(asJSON bool) flow.IO {
	return flow.IO{Ctx: d.ctx(), Tool: d.Tool, Stdout: d.stdout(), Stderr: d.stderr(), JSON: asJSON}
}

func (d *Deps) ctx() context.Context {
	if d.Context != nil {
		return d.Context
	}
	return context.Background()
}

func (d *Deps) stdout() io.Writer {
	if d.Stdout != nil {
		return d.Stdout
	}
	return os.Stdout
}

func (d *Deps) stderr() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
}

// VersionCmd prints the running version and, with --check, the latest
// published release.
type VersionCmd struct {
	JSON  bool `help:"Write one JSON document." name:"json"`
	Check bool `help:"Also look up the latest published release."`
}

// Run implements the command.
func (c *VersionCmd) Run(d *Deps) error {
	return flow.Version(d.io(c.JSON), d.Version, d.Build, c.Check, d.Checker) //nolint:wrapcheck // flow errors carry context
}

// ChangelogCmd shows release notes, embedded first, then releases
// published since this build.
type ChangelogCmd struct {
	JSON        bool   `help:"Write one JSON document." name:"json"`
	Since       string `help:"Only releases newer than this version."`
	Limit       int    `help:"Show at most this many releases." default:"10"`
	Prereleases bool   `help:"Include prereleases."`
}

// Run implements the command.
func (c *ChangelogCmd) Run(d *Deps) error {
	return flow.Changelog(d.io(c.JSON), d.Changelog, update.CollectOptions{ //nolint:wrapcheck // flow errors carry context
		Since: c.Since, Limit: c.Limit, Prereleases: c.Prereleases,
	})
}

// UpdateCmd replaces the running binary with the latest release.
type UpdateCmd struct {
	JSON  bool `help:"Write one JSON document." name:"json"`
	Check bool `help:"Only report whether an update is available."`
}

// ErrUpdateUnavailable is returned when no updater is wired or the
// release source cannot run; its errkind maps to exitcode.Unavailable.
var ErrUpdateUnavailable error = errkind.New(errkind.Unavailable, "self-update is unavailable for this build")

// Run implements the command.
func (c *UpdateCmd) Run(d *Deps) error {
	return flow.Update(d.io(c.JSON), d.Version, c.Check, d.Checker, d.Updater, ErrUpdateUnavailable) //nolint:wrapcheck // flow errors carry context
}
