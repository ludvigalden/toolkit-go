# Release awareness

A tool knows its version and release notes from its own repository's
relmap declarations, and learns about newer releases from GitHub through
`gh` when it is installed.

## Embedding the export

Declare a class for the generated files in `map/` so relmap tracks them
as generated, then add to the package that embeds them:

```go
//go:generate sh -c "cd <repo root> && relmap export --out <dir>/data --target ndjson,index,metadata && relmap version . --json > <dir>/data/version.json"

//go:embed data
var data embed.FS
```

`relmapx.Load(fsys, ".")` reads the export (only `changelog.ndjson` is
required) and `relmapx.LoadBuildState(fsys, "version.json")` the build
state. Commit the generated files so a clean clone builds. Unknown
fields are ignored; an export whose schema is not 1 fails with
`relmapx.ErrSchema`.

`BuildState.Version()` is the version the build presents.
`Presented` carries relmap's marker: `+unknown` when relmap could not
judge the build (an unreleased change, an undeclared commit, or no git
at generation time) and `+dirty` when forced on the build machine.

## Composing the surfaces

| Surface      | Compose                                                              | Without `gh`                                                                         |
| ------------ | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| version      | `kongcmd.VersionCmd` with `Deps.Build`                               | works                                                                                |
| changelog    | `update.Changelog{Embedded, Line, Remote: ghcli.New(repo)}`          | embedded releases only; `View.RemoteUnavailable`                                     |
| update check | `ghcli.New(repo)` as `update.Checker`                                | `update.ErrUnavailable`                                                              |
| self-update  | `selfupdate.New(selfupdate.Config{Repo})` as `update.Updater`        | uses the GitHub API directly; `kongcmd` reports unavailable when no updater is wired |
| notice       | `update.Notice{Tool, Current, Checker}`; call `Show` after a command | silent                                                                               |

The notice checks at most once per `TTL` (default 24h), caching under
the user cache directory, and is off when `CI` is set, stderr is not a
terminal, or `<TOOL>_NO_UPDATE_NOTIFIER` is set. A failing remote never
fails `changelog` while embedded entries exist; it is reported once on
stderr.

## Publishing

Release on a `v<version>` tag. Attach the binaries named
`<tool>_<version>_<os>_<arch>` and a `checksums.txt` (sha256), and use
the tagged version's relmap rendering as the release description:

```sh
relmap export --out notes --target markdown --tree
gh release create "v$VERSION" dist/* --notes-file "notes/changelog/<line>/$VERSION.md"
```

`examples/go_tool_toolname/.github/workflows/release.yml` is a complete
workflow.
