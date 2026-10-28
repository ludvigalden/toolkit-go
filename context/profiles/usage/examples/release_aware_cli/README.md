# release_aware_cli

The smallest Go binary that knows its own release history. `shipit`
embeds a relmap changelog export and composes toolkit-go's update
interfaces by hand:

| Verb | Interface | Without `gh` |
| --- | --- | --- |
| `shipit version` | `relmapx.BuildState` (embedded `relmap version --json`) | works |
| `shipit notes` | `update.Changelog` (embedded export + `ghcli` for newer releases) | embedded notes only |
| `shipit check` | `update.Checker` (`ghcli`) | exits 69, prints nothing |

## How the changelog gets into the binary

1. Changes and releases are declared under `changelog/`, the map under
   `map/` (relmap's two declaration families).
2. `go generate ./...` runs `relmap export --target ndjson,index,metadata`
   and `relmap version . --json` into
   `tools/shipit/internal/app/relnotes/data/`.
3. `relnotes` embeds that directory with `go:embed`; `relmapx` reads it.

The export is committed so `go build` works from a clean clone. In this
example tree it presents `1.0.0+unknown`: the export was generated
outside a git repository, so relmap could not prove the build clean.

## Releasing

`.github/workflows/release.yml` builds on a `v*` tag, checks changelog
coverage, and creates a GitHub release whose description is the relmap
rendering of that one version (`relmap export --target markdown --tree`)
with a `checksums.txt` beside the binaries — what `ghcli` reads for
notes and what `selfupdate` verifies downloads against.

## Run it

```sh
go run ./tools/shipit/cmd version
go run ./tools/shipit/cmd notes
go run ./tools/shipit/cmd check
```
