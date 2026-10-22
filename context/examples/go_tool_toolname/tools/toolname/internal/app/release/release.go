// Package release embeds the relmap export of this repository's
// changelog and the build state relmap computed for it, so the binary
// knows its own version and release notes without a network.
package release

import (
	"embed"
	"io/fs"

	"github.com/ludvigalden/toolkit-go/pkg/update/relmapx"
)

// Regenerate after editing changelog/ (run from the repository root):
//
//go:generate sh -c "cd ../../../../.. && relmap export --out tools/toolname/internal/app/release/data --target ndjson,index,metadata && relmap version . --json > tools/toolname/internal/app/release/data/version.json"

//go:embed data
var data embed.FS

func sub() fs.FS {
	f, err := fs.Sub(data, "data")
	if err != nil {
		panic(err) // the embed directive guarantees the directory
	}
	return f
}

// Changelog returns the embedded relmap export.
func Changelog() (*relmapx.Changelog, error) { return relmapx.Load(sub(), ".") }

// Build returns the embedded `relmap version --json` state.
func Build() (relmapx.BuildState, error) { return relmapx.LoadBuildState(sub(), "version.json") }
