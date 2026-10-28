// Package app is shipit's wiring: three verbs, each composed from one
// small toolkit-go interface.
package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/ghcli"
	"github.com/ludvigalden/toolkit-go/pkg/update/relmapx"

	"example.com/shipit/tools/shipit/internal/app/relnotes"
)

const (
	tool = "shipit"
	repo = "example/shipit"
)

// Main runs one verb: version, notes, or check.
func Main(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: shipit version | notes | check")
		return int(exitcode.Usage)
	}
	cl, build, err := relnotes.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, tool+":", err)
		return int(exitcode.Software)
	}
	ctx := context.Background()
	var checker update.Checker = ghcli.New(repo)
	switch args[0] {
	case "version":
		return version(os.Stdout, build)
	case "notes":
		return notes(ctx, os.Stdout, cl, ghcli.New(repo))
	case "check":
		return check(ctx, os.Stdout, build.Version(), checker)
	default:
		fmt.Fprintln(os.Stderr, "usage: shipit version | notes | check")
		return int(exitcode.Usage)
	}
}

// version prints the presented version; "+unknown" marks a build relmap
// could not judge clean (undeclared commits, or no git at build time).
func version(w io.Writer, b relmapx.BuildState) int {
	fmt.Fprintln(w, b.Presented)
	if !b.Clean() {
		fmt.Fprintf(os.Stderr, "%s: this build is not a clean %s release\n", tool, b.Base)
	}
	return int(exitcode.OK)
}

// notes shows the embedded changelog, preceded by any newer releases gh
// can see; without gh it is the embedded changelog alone.
func notes(ctx context.Context, w io.Writer, cl *relmapx.Changelog, remote update.NotesSource) int {
	c := &update.Changelog{Embedded: cl, Line: tool, Remote: remote}
	v, err := c.Collect(ctx, update.CollectOptions{Limit: 5})
	if err != nil {
		fmt.Fprintln(os.Stderr, tool+":", err)
		return int(exitcode.Failure)
	}
	if err := c.Render(w, v); err != nil {
		return int(exitcode.Failure)
	}
	return int(exitcode.OK)
}

// check reports whether a newer release exists; unavailable (no gh) is
// a distinct exit code, never a failure message.
func check(ctx context.Context, w io.Writer, current string, c update.Checker) int {
	latest, err := c.Latest(ctx)
	switch {
	case update.IsUnavailable(err):
		return int(exitcode.Unavailable)
	case err != nil:
		fmt.Fprintln(os.Stderr, tool+":", err)
		return int(exitcode.Failure)
	case update.Newer(latest.Version, current):
		fmt.Fprintf(w, "%s is available (running %s)\n", latest.Version, current)
	default:
		fmt.Fprintf(w, "%s is the latest release\n", current)
	}
	return int(exitcode.OK)
}
