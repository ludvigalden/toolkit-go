# go_tool_toolname

The layout every public Go tool repository follows, as one complete,
buildable tool named `toolname`.

```text
tools/toolname/cmd/main.go            binary entry point (os.Args -> app.Main)
tools/toolname/internal/app/**        binary-internal wiring: grammar, commands, embeds
pkg/go/toolname/*.go                  tool-wide public API (names, defaults)
pkg/go/toolname/catalog/*.go          specialized public API
pkg/go/toolname/catalog/internal/**   its internals
pkg/go/common/textfmt/*.go            repository-common public API
pkg/go/common/textfmt/internal/**     its internals
map/  changelog/                      relmap declarations: what ships, what changed
```

`cmd` stays a few lines so every behaviour is reachable from
`internal/app`, which tests and other entry points can call directly.
Anything another repository may import lives under `pkg/go/`; anything
that may change freely lives under an `internal/` directory.

## Release awareness

The binary composes toolkit-go's update stack:

| Command | Built from | Without `gh` on PATH |
| --- | --- | --- |
| `toolname version` | `kongcmd.VersionCmd` + the embedded `relmap version --json` | works |
| `toolname changelog` | `kongcmd.ChangelogCmd` + the embedded relmap export, then `ghcli` for releases newer than the build | embedded notes only |
| `toolname update` | `kongcmd.UpdateCmd` + `selfupdate` (go-selfupdate, checksum-verified) | unavailable, exit 69 |
| notice after `list`/`show` | `update.Notice`, cached for a day; off in CI, in pipes, or with `TOOLNAME_NO_UPDATE_NOTIFIER=1` | silent |

The changelog is authored with relmap under `changelog/`; `go generate
./...` refreshes the embedded export in
`tools/toolname/internal/app/release/data/`. The release workflow in
`.github/workflows/release.yml` publishes each tag with that version's
relmap-rendered notes and a `checksums.txt`.

## Run it

```sh
go run ./tools/toolname/cmd list
go run ./tools/toolname/cmd --stdout-format=json show widget
go run ./tools/toolname/cmd version
go run ./tools/toolname/cmd changelog
```
