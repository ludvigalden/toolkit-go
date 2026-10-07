# minimal_cli

A small note-keeping CLI, `notes`, built on toolkit-go's CLI stack:

- `common/cli` — `cli.App` parses the Kong grammar (with the shared
  `GlobalFlags`: `--stdout-format`, `--stdout-verbosity`, `--config`,
  …), layers configuration, and runs the command with its output,
  logger, and config bound. The tool's own key `notes.dir` gets a
  default through `App.Defaults` and is overridable from `notes.yaml`
  (searched from the working directory up to `$HOME`), `NOTES_DIR`, or
  `--config` files.
- `common/output` — the result is the stdout payload in the chosen mode
  (text, json, ndjson, yamlish); diagnostics go to stderr.
- Exit codes follow sysexits: a returned `errkind` error picks its own.

Layout: `tools/notes/cmd/main.go` (entry point),
`tools/notes/internal/app` (wiring), `pkg/go/notes` (the store, the
tool's public API).

```sh
go run ./tools/notes/cmd add buy milk
go run ./tools/notes/cmd list --stdout-format=json
NOTES_DIR=/tmp/notes go run ./tools/notes/cmd list
```
