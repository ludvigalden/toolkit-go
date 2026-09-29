// Command ghcli_smoke checks gh-backed releases with gh absent (every
// call is ErrUnavailable, nothing printed) and present (reads the latest
// release of GH_SMOKE_REPO, default cli/cli; skipped without auth).
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/ghcli"
)

var failures int

func check(ok bool, format string, args ...any) {
	if ok {
		fmt.Printf("ok   %s\n", fmt.Sprintf(format, args...))
		return
	}
	failures++
	fmt.Printf("FAIL %s\n", fmt.Sprintf(format, args...))
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo := os.Getenv("GH_SMOKE_REPO")
	if repo == "" {
		repo = "cli/cli"
	}

	// Locate gh before stripping PATH so the working path can still run.
	ghPath, ghErr := exec.LookPath("gh")

	absent(ctx, repo)

	if ghErr != nil {
		fmt.Println("skip working path: gh is not on PATH")
	} else if err := exec.CommandContext(ctx, ghPath, "auth", "status").Run(); err != nil {
		fmt.Println("skip working path: gh is not authenticated")
	} else {
		working(ctx, repo)
	}

	if failures > 0 {
		fmt.Printf("%d check(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

func absent(ctx context.Context, repo string) {
	saved := os.Getenv("PATH")
	empty, _ := os.MkdirTemp("", "nogh-*")
	defer func() { _ = os.RemoveAll(empty) }()
	_ = os.Setenv("PATH", empty)
	defer func() { _ = os.Setenv("PATH", saved) }()

	check(!ghcli.Available(), "gh absent: Available reports false")
	src := ghcli.New(repo)
	_, err := src.Latest(ctx)
	check(errors.Is(err, update.ErrUnavailable), "gh absent: Latest returns ErrUnavailable (err %v)", err)
	_, err = src.Releases(ctx, 5)
	check(errors.Is(err, update.ErrUnavailable), "gh absent: Releases returns ErrUnavailable")

	// A notice and a changelog with only a remote source stay silent.
	var out bytes.Buffer
	n := &update.Notice{
		Tool: "demo", Current: "0.0.1", Checker: src, Out: &out,
		CacheDir: empty, Interactive: func() bool { return true },
	}
	check(!n.Show(ctx) && out.Len() == 0, "gh absent: notice prints nothing")
	cl := &update.Changelog{Line: "demo", Remote: src}
	v, err := cl.Collect(ctx, update.CollectOptions{})
	check(err == nil && v.RemoteUnavailable && len(v.Entries) == 0, "gh absent: changelog is empty and marks remote unavailable, no error")
}

func working(ctx context.Context, repo string) {
	src := ghcli.New(repo)
	rel, err := src.Latest(ctx)
	check(err == nil && rel.Tag != "" && rel.Version != "" && rel.URL != "" && !rel.PublishedAt.IsZero(),
		"gh present: latest release of %s is %s (err %v)", repo, rel.Tag, err)
	check(rel.Notes != "", "gh present: latest release carries its description (%d bytes)", len(rel.Notes))

	rels, err := src.Releases(ctx, 5)
	check(err == nil && len(rels) > 0, "gh present: release list returns %d entries", len(rels))

	if len(rels) >= 2 {
		since := rels[1].Version
		newer, err := src.ReleasesNewerThan(ctx, since, 5)
		check(err == nil && len(newer) >= 1 && newer[0].Notes != "", "gh present: releases newer than %s come with notes", since)
	}
}
