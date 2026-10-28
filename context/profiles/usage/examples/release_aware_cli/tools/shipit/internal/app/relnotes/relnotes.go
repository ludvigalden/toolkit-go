// Package relnotes embeds the relmap export written by `go generate`.
package relnotes

import (
	"embed"
	"io/fs"

	"github.com/ludvigalden/toolkit-go/pkg/update/relmapx"
)

//go:generate sh -c "cd ../../../../.. && relmap export --out tools/shipit/internal/app/relnotes/data --target ndjson,index,metadata && relmap version . --json > tools/shipit/internal/app/relnotes/data/version.json"

//go:embed data
var data embed.FS

func dataFS() fs.FS {
	sub, err := fs.Sub(data, "data")
	if err != nil {
		panic(err) // guaranteed by the embed directive
	}
	return sub
}

// Load returns the embedded changelog and build state.
func Load() (*relmapx.Changelog, relmapx.BuildState, error) {
	cl, err := relmapx.Load(dataFS(), ".")
	if err != nil {
		return nil, relmapx.BuildState{}, err
	}
	st, err := relmapx.LoadBuildState(dataFS(), "version.json")
	return cl, st, err
}
