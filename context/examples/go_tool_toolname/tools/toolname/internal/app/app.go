// Package app wires toolname: the Kong grammar, the shared CLI flags,
// and the update capabilities the binary composes from toolkit-go.
package app

import (
	"context"
	"io"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/cli"
	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/ghcli"
	"github.com/ludvigalden/toolkit-go/pkg/update/kongcmd"
	"github.com/ludvigalden/toolkit-go/pkg/update/selfupdate"

	"example.com/toolname/pkg/go/toolname"
	"example.com/toolname/pkg/go/toolname/catalog"
	"example.com/toolname/tools/toolname/internal/app/commands"
	"example.com/toolname/tools/toolname/internal/app/release"
)

// Root is the Kong grammar. The update commands are toolkit-go's; the
// tool only decides which of them it exposes.
type Root struct {
	cli.GlobalFlags

	List commands.ListCmd `cmd:"" help:"List items."`
	Show commands.ShowCmd `cmd:"" help:"Show one item."`

	Version   kongcmd.VersionCmd   `cmd:"" help:"Show the version this build presents."`
	Changelog kongcmd.ChangelogCmd `cmd:"" help:"Show release notes."`
	Update    kongcmd.UpdateCmd    `cmd:"" help:"Replace this binary with the latest release."`
}

// Main runs toolname with args and returns the process exit code.
func Main(args []string, stamped string) int {
	return run(args, stamped, os.Stdout, os.Stderr, os.Getenv)
}

func run(args []string, stamped string, stdout, stderr io.Writer, getenv func(string) string) int {
	app := &cli.App{
		Name: toolname.Name, Description: "List and show catalog items.",
		Stdout: stdout, Stderr: stderr, Getenv: getenv,
	}
	r, code := app.Parse(&Root{}, args)
	if r == nil {
		return code
	}
	ctx := context.Background()
	deps := updateDeps(ctx, stamped, stdout, stderr)
	app.Version = deps.Version

	// The notice is opt-out (TOOLNAME_NO_UPDATE_NOTIFIER, CI, pipes) and
	// cached for a day, so normal commands rarely touch the network.
	if cmd := r.Kong.Command(); cmd == "list" || cmd == "show <name>" {
		notice := &update.Notice{Tool: toolname.Name, Current: deps.Version, Checker: deps.Checker, Out: stderr, Getenv: getenv}
		defer notice.Show(ctx)
	}
	// Interfaces bind with BindTo. A returned error picks the exit code
	// by its errkind: no update source exits 69, an unknown item 66.
	r.Kong.BindTo(catalog.Builtin(), (*catalog.Catalog)(nil))
	return r.Exec(deps)
}

// updateDeps composes the update stack. Every capability is optional:
// without gh the remote notes and checks are simply unavailable, and
// the embedded changelog still works offline.
func updateDeps(ctx context.Context, stamped string, stdout, stderr io.Writer) *kongcmd.Deps {
	d := &kongcmd.Deps{Tool: toolname.Name, Version: stamped, Context: ctx, Stdout: stdout, Stderr: stderr}
	if b, err := release.Build(); err == nil {
		d.Build = &b
		if d.Version == "" {
			d.Version = b.Version()
		}
	}
	embedded, _ := release.Changelog() //nolint:errcheck // a build without an export just has no embedded notes
	gh := ghcli.New(toolname.Repo)
	d.Changelog = &update.Changelog{Embedded: embedded, Line: toolname.Line, Remote: gh}
	d.Checker = gh
	if up, err := selfupdate.New(selfupdate.Config{Repo: toolname.Repo}); err == nil {
		d.Updater = up
	}
	return d
}
