// Command compose checks the update composition with fakes: notice
// throttling and opt-outs, the embedded-plus-remote changelog, and the
// Kong command payloads. TESTDATA points at testdata/relmap-tern.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/update"
	"github.com/ludvigalden/toolkit-go/pkg/update/kongcmd"
	"github.com/ludvigalden/toolkit-go/pkg/update/relmapx"
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

type fakeChecker struct {
	rel   update.Release
	err   error
	calls int
}

func (f *fakeChecker) Latest(context.Context) (update.Release, error) {
	f.calls++
	return f.rel, f.err
}

type fakeNotes struct{ rels []update.Release }

func (f fakeNotes) ReleasesNewerThan(_ context.Context, v string, _ int) ([]update.Release, error) {
	var out []update.Release
	for _, r := range f.rels {
		if update.Newer(r.Version, v) {
			out = append(out, r)
		}
	}
	return out, nil
}

type fakeUpdater struct{ to string }

func (f fakeUpdater) Apply(_ context.Context, cur string) (update.Release, bool, error) {
	return update.Release{Version: f.to}, update.Newer(f.to, cur), nil
}

func main() {
	ctx := context.Background()
	_ = os.Unsetenv("CI")
	_ = os.Unsetenv("DEMO_TOOL_NO_UPDATE_NOTIFIER")

	check(update.Newer("1.10.0", "1.9.0") && !update.Newer("1.0.0-rc.1", "1.0.0") && update.Newer("v2.0.0", "1.9.9"), "semver ordering")
	check(update.VersionFromTag("v1.2.3") == "1.2.3" && update.VersionFromTag("vendor") == "vendor", "tag to version")

	// Notice: cache and throttle.
	dir, _ := os.MkdirTemp("", "notice-*")
	defer func() { _ = os.RemoveAll(dir) }()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	fc := &fakeChecker{rel: update.Release{Version: "1.3.0", URL: "https://example.invalid/r"}}
	var out bytes.Buffer
	n := &update.Notice{
		Tool: "demo-tool", Current: "1.2.0", Checker: fc, CacheDir: dir, Out: &out,
		Interactive: func() bool { return true }, Now: func() time.Time { return now },
	}
	check(n.Show(ctx) && strings.Contains(out.String(), "version 1.3.0 is available (running 1.2.0)"), "notice prints one line: %q", strings.TrimSpace(out.String()))
	n.Show(ctx)
	check(fc.calls == 1, "second notice within TTL uses the cache (calls %d)", fc.calls)
	now = now.Add(25 * time.Hour)
	n.Show(ctx)
	check(fc.calls == 2, "notice re-checks after the TTL (calls %d)", fc.calls)

	out.Reset()
	_ = os.Setenv("DEMO_TOOL_NO_UPDATE_NOTIFIER", "1")
	check(!n.Show(ctx) && out.Len() == 0, "<TOOL>_NO_UPDATE_NOTIFIER disables the notice")
	_ = os.Unsetenv("DEMO_TOOL_NO_UPDATE_NOTIFIER")
	_ = os.Setenv("CI", "true")
	check(!n.Show(ctx), "CI disables the notice")
	_ = os.Unsetenv("CI")
	n.Interactive = func() bool { return false }
	check(!n.Show(ctx), "non-terminal disables the notice")
	n.Interactive = func() bool { return true }

	failing := &update.Notice{
		Tool: "demo-tool", Current: "1.2.0", CacheDir: dir + "/x", Out: &out,
		Checker: &fakeChecker{err: errors.New("network down")}, Interactive: func() bool { return true },
	}
	out.Reset()
	check(!failing.Show(ctx) && out.Len() == 0, "a failing checker prints nothing")

	// Changelog: embedded relmap export plus remote notes newer than it.
	emb, err := relmapx.Load(os.DirFS(os.Getenv("TESTDATA")), ".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	cl := &update.Changelog{Embedded: emb, Line: "es-runtime", Remote: fakeNotes{rels: []update.Release{
		{Version: "1.3.0", Name: "v1.3.0", Notes: "## Added\n- Cancel streams", PublishedAt: now},
		{Version: "1.2.0", Notes: "already embedded"},
	}}}
	v, err := cl.Collect(ctx, update.CollectOptions{})
	check(err == nil && len(v.Entries) == 2 && v.Entries[0].Origin == "remote" && v.Entries[0].Version == "1.3.0" &&
		v.Entries[1].Origin == "embedded" && v.Entries[1].Version == "1.2.0", "changelog = remote 1.3.0 then embedded 1.2.0 (%+v)", summarize(v))
	var text bytes.Buffer
	_ = cl.Render(&text, v)
	check(strings.Contains(text.String(), "es-runtime 1.3.0") && strings.Contains(text.String(), "- Cancel streams") &&
		strings.Contains(text.String(), "es-runtime 1.2.0 (2026-10-10)"), "changelog renders both origins")
	v, _ = cl.Collect(ctx, update.CollectOptions{Since: "1.2.0"})
	check(len(v.Entries) == 1 && v.Entries[0].Version == "1.3.0", "--since filters embedded entries")

	// Kong commands: payloads on stdout.
	var stdout, stderr bytes.Buffer
	d := &kongcmd.Deps{
		Tool: "demo-tool", Version: "1.2.0", Changelog: cl, Checker: fc,
		Updater: fakeUpdater{to: "1.3.0"}, Stdout: &stdout, Stderr: &stderr,
	}
	err = (&kongcmd.VersionCmd{JSON: true, Check: true}).Run(d)
	var vp map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &vp)
	check(err == nil && vp["version"] == "1.2.0" && vp["up_to_date"] == false, "version --json --check payload: %s", strings.TrimSpace(stdout.String()))
	stdout.Reset()
	err = (&kongcmd.ChangelogCmd{JSON: true, Limit: 10}).Run(d)
	check(err == nil && strings.Contains(stdout.String(), `"origin": "remote"`), "changelog --json payload carries origins")
	stdout.Reset()
	stderr.Reset()
	err = (&kongcmd.UpdateCmd{}).Run(d)
	check(err == nil && stdout.Len() == 0 && strings.Contains(stderr.String(), "updated 1.2.0 -> 1.3.0"), "update reports on stderr only: %q", strings.TrimSpace(stderr.String()))
	err = (&kongcmd.UpdateCmd{}).Run(&kongcmd.Deps{Tool: "demo-tool", Version: "1.2.0", Stdout: &stdout, Stderr: &stderr})
	check(errors.Is(err, kongcmd.ErrUpdateUnavailable), "update without an updater is unavailable")

	if failures > 0 {
		fmt.Printf("%d check(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

func summarize(v update.View) []string {
	var s []string
	for _, e := range v.Entries {
		s = append(s, e.Origin+":"+e.Version)
	}
	return s
}
