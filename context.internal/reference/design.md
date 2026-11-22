# Design decisions

The decisions the module was shaped by, for whoever changes it. What
each package does is in the `usage` profile of `context/` and in the
packages' own documentation.

## Shape

- toolkit-go is one library module, not a monorepo: its public
  packages sit under `pkg/` at the module root and are imported as
  `github.com/ludvigalden/toolkit-go/pkg/<name>`. The layout of a tool
  repository (`tools/<tool>/cmd`, `tools/<tool>/internal/app`,
  `pkg/go/<tool>`, `pkg/go/common/<name>`) is shown by
  `context/examples/go_tool_toolname/`, not followed by this module.
- Lean, well-abstracted packages and no compatibility shims. A
  consumer moving to a package rewrites its call sites to the new API
  and passes its own values (lock roots, environment variable names,
  state directories) explicitly.
- No package-level mutable state. Configuration is a value; the
  environment is read through an injected `getenv`, and a directory a
  package needs (a lock root, a state home) is an explicit option.
- Exit codes follow sysexits; `errkind` maps each error kind to one.

## Dependencies

- Packages need only the standard library and `golang.org/x/{sys,term}`,
  plus Kong and koanf for `cli`, `x/crypto` for `cryptobox` and `x/net`
  for `hostnorm`.
- `update/selfupdate` is the one heavy package, and nothing else in
  the module imports it.
- No package depends on a terminal-UI library. The Bubble Tea renderer
  lives in `draft/terminal`, a separate Go module.
- `procrun` reads process statistics from `/proc` on Linux and from
  `ps` on macOS rather than through a cross-platform statistics
  library.
- No dependency on any private module, and no repository-index client.

## Release awareness

- Changelogs are authored with relmap; toolkit-go only reads its
  exports (`changelog.ndjson`, `changelog.index.json`,
  `changelog.metadata.json`) through `update/relmapx`.
- Release notes and update notices go through the `gh` CLI as a
  subprocess. Without `gh` on `PATH` those surfaces are simply
  unavailable: no HTTP fallback, and nothing printed.

## Enumeration

- `fsenum` takes its skips from categorised default sets (`Set`,
  `Default()`, the VCS to legacy categories, and the opt-in generated
  code, image and agent-configuration sets), covering files as well as
  directories; a caller supplies or extends the set.
