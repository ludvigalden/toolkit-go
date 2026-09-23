// Command relmapx_read checks the relmap export reader against a real
// export of relmap's "tern" fixture (testdata/relmap-tern) and the
// saved `relmap version --json` output beside it.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing/fstest"

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

func testdata() string {
	if d := os.Getenv("TESTDATA"); d != "" {
		return d
	}
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata", "relmap-tern")
}

func main() {
	dir := testdata()
	c, err := relmapx.Load(os.DirFS(dir), ".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}

	h := c.Header()
	check(h.Schema == 1, "header schema is 1 (got %d)", h.Schema)
	check(len(h.Categories) == 9 && h.Categories[0] == "breaking", "header lists the nine categories")
	check(h.Scope.IncludePending, "header records include_pending")
	check(c.Index() != nil && c.Index().NDJSON.Lines == 25, "index loaded and covers 25 lines")
	check(c.Metadata() != nil && len(c.Metadata().Lines) > 0, "metadata loaded")

	lines := c.Lines()
	check(strings.Join(lines, ",") == "api,es-runtime,go-sdk,notify,site-docs,tern", "lines are sorted: %v", lines)

	tern := c.Releases("tern")
	check(len(tern) >= 2 && tern[0].Version == "0.4.0-rc.2", "tern releases newest first (got %s)", first(tern))

	cur, ok := c.Current("tern")
	check(ok && cur.Version == "0.4.0-rc.2" && !cur.Stable, "tern current release is the rc (got %s)", cur.Version)

	newer := c.Newer("tern", "0.3.0", false)
	check(len(newer) == 0, "no stable tern release newer than 0.3.0 (got %d)", len(newer))
	newerPre := c.Newer("tern", "0.3.0", true)
	check(len(newerPre) >= 1 && newerPre[0].Version == "0.4.0-rc.2", "prerelease newer than 0.3.0 is rc.2")

	rel, ok := c.Release("es-runtime@1.2.0")
	check(ok && rel.Promotes == "es-runtime@1.2.0-beta.1" && len(rel.Sections) == 2, "es-runtime@1.2.0 decoded with promotes and two sections")
	check(len(rel.Compat) == 1 && rel.Compat[0].Range == ">=0.3.0 <0.5.0", "compat range decoded")

	api, ok := c.Release("api@2.7.0")
	hasNotes := false
	for _, comp := range api.Components {
		if comp.Notes != nil && comp.Notes.Paragraph != "" {
			hasNotes = true
		}
	}
	check(ok && hasNotes, "api@2.7.0 carries curated component notes")

	pending := c.Changes("")
	check(len(pending) >= 3, "pending changes present (got %d)", len(pending))
	inRC := c.Changes("tern@0.4.0-rc.2")
	check(len(inRC) >= 1, "changes included by tern@0.4.0-rc.2 (got %d)", len(inRC))
	check(len(c.ChangesFor("services/api")) >= 1, "changes reaching services/api found")

	var out bytes.Buffer
	if err := c.Render(&out, c.Releases("es-runtime"), relmapx.RenderOptions{Changes: true}); err != nil {
		check(false, "render: %v", err)
	}
	text := out.String()
	check(strings.Contains(text, "es-runtime 1.2.0 (2026-10-10)") && strings.Contains(text, "Added") &&
		strings.Contains(text, "- Stream results as tern produces them"), "render produces heading, category and item")

	vb, err := os.ReadFile(filepath.Join(dir, "version.json"))
	check(err == nil, "version.json readable")
	bs, err := relmapx.ParseBuildState(vb)
	check(err == nil && bs.Line == "api" && bs.Base == "2.7.0", "build state decoded (err %v)", err)
	check(bs.Version() == "2.7.0" && !bs.Clean() && bs.State.ShortHash != "", "build state presents 2.7.0, unknown, with short hash")
	check(bs.OlderThan("2.8.0") && !bs.OlderThan("2.7.0"), "build state compares by base version")

	// Forward compatibility: unknown fields and kinds are ignored.
	fwd := `{"kind":"header","schema":1,"categories":[],"scope":{},"state":{},"future":1}
{"kind":"release","id":"x@1.0.0","line":"x","version":"1.0.0","stable":true,"new_field":{"a":1}}
{"kind":"milestone","id":"m1"}
`
	fc, err := relmapx.ParseNDJSON(strings.NewReader(fwd))
	check(err == nil && len(fc.Releases("x")) == 1, "unknown fields and record kinds tolerated (err %v)", err)

	// Schema guard.
	_, err = relmapx.ParseNDJSON(strings.NewReader(`{"kind":"header","schema":2}` + "\n"))
	check(errors.Is(err, relmapx.ErrSchema), "schema 2 rejected with ErrSchema (err %v)", err)
	_, err = relmapx.ParseBuildState([]byte(`{"schema":3,"presented":"1.0.0"}`))
	check(errors.Is(err, relmapx.ErrSchema), "build state schema 3 rejected")

	// go:embed-style loading: only the ndjson present.
	raw, _ := os.ReadFile(filepath.Join(dir, relmapx.NDJSONFile))
	mfs := fstest.MapFS{"export/" + relmapx.NDJSONFile: {Data: raw}}
	ec, err := relmapx.Load(mfs, "export")
	check(err == nil && ec.Index() == nil && len(ec.Lines()) == 6, "loads from an fs.FS with only the ndjson (err %v)", err)

	if failures > 0 {
		fmt.Printf("%d check(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

func first(r []relmapx.Release) string {
	if len(r) == 0 {
		return "<none>"
	}
	return r[0].Version
}
