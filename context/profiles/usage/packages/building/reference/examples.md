# Examples

Each example is a standalone Go module that builds with `go build ./...`
from its own directory.

| Example | Shows |
| --- | --- |
| `examples/go_tool_toolname` (module root) | the canonical repository layout with `kongcmd` version, changelog and update commands, the update notice, and a release workflow |
| `profiles/usage/examples/minimal_cli` | `cli.App` (tool config keys with defaults), `output` modes, `errkind` exit codes |
| `profiles/usage/examples/process_runner` | `procrun` (timeouts, streaming, a custom `Scoper`, `ReadStats`), `proclock` |
| `profiles/usage/examples/repo_walker` | `fsenum` (gitignore-honoring walk, caller-supplied skip) |
| `profiles/usage/examples/release_aware_cli` | the update interfaces composed by hand, an embedded relmap export, and the release workflow |
