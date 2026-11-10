# toolkit-go

Shared Go packages for command-line tools: Kong CLI wiring, output
modes (data on stdout, diagnostics on stderr), layered configuration,
cross-process locks, filesystem enumeration and file classification, a
subprocess runner, and a composable update stack — self-update, release
notes and update notices — that reads changelog exports from relmap.

```sh
go get github.com/ludvigalden/toolkit-go
```

Linux and macOS; Go 1.26.

## Packages

Public packages live under `pkg/`, imported as
`github.com/ludvigalden/toolkit-go/pkg/<name>`; implementation detail
sits in `internal/` directories beside them:

| Package                                                                           | For                                                                                                              |
| --------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `cli`, `output`, `exitcode`, `errkind`, `diag`                                    | a Kong CLI with shared flags and a config cascade, output modes, exit codes, error kinds, positioned diagnostics |
| `xdg`, `dotenv`, `secret`, `severity`, `yamlish`                                  | standard directories, dotenv files, redacted secrets, log severities, a YAML-like text formatter                 |
| `procrun`, `proclock`, `procserve`, `diskfree`                                    | subprocesses, cross-process locks, warm background servers, free space                                           |
| `supervisor`                                                                      | OS service management via systemd-user, launchd, or setsid                                                       |
| `fsenum`                                                                          | gitignore-honoring repository enumeration with categorized default skip sets                                     |
| `serve`                                                                           | HTTP server lifecycle: graceful shutdown, health endpoints, kill escalation                                      |
| `scratch`                                                                         | hermetic scratch environments for tool tests                                                                     |
| `cryptobox`                                                                       | passphrase and raw-key authenticated encryption of byte streams                                                  |
| `ssrf`, `urlnorm`, `hostnorm`                                                     | fetcher URL validation, tracking-param normalization, eTLD+1 host keying                                         |
| `untrusted`                                                                       | nonce-delimited fencing of untrusted content in prompts                                                          |
| `update`, `update/relmapx`, `update/ghcli`, `update/selfupdate`, `update/kongcmd` | release awareness                                                                                                |

No package keeps mutable package-level state; configuration is passed
as values, and the environment is injected where it is read. Packages
need only the standard library and `golang.org/x/{sys,term}` (plus Kong
and koanf for `cli`, `x/crypto` for `cryptobox`, `x/net` for
`hostnorm`); `update/selfupdate` is the one heavy package and
nothing else imports it. No package depends on a terminal-UI library,
so all of them work inside interactive applications; an experimental
Bubble Tea renderer lives in `draft/terminal` as a separate module.

## Release awareness

A tool embeds its own relmap changelog export and composes only the
capabilities it wants:

```go
gh := ghcli.New("owner/tool")                       // absent gh => ErrUnavailable, never noise
cl := &update.Changelog{Embedded: export, Line: "tool", Remote: gh}
up, _ := selfupdate.New(selfupdate.Config{Repo: "owner/tool"}) // checksums.txt verified
deps := &kongcmd.Deps{Tool: "tool", Changelog: cl, Checker: gh, Updater: up}
```

`kongcmd` provides ready `version`, `changelog` and `update` commands.

## Layout and examples

`context/examples/go_tool_toolname/` is a complete tool in the layout
public tool repositories follow (`tools/<tool>/cmd`,
`tools/<tool>/internal/app`, `pkg/go/<tool>`, `pkg/go/common`), with a
relmap changelog and a release workflow. Smaller examples live under
`context/profiles/usage/examples/`. Each is its own module:

```sh
cd context/examples/go_tool_toolname && go run ./tools/toolname/cmd changelog
```

## Tests

Packages carry unit tests (`go test ./pkg/...`); end-to-end checks are
programs under `tests/`:

```sh
go run ./tests/examples/build                                   # build and vet every example
TESTDATA=$PWD/tests/update/testdata/relmap-tern go run ./tests/update/relmapx_read
TESTDATA=$PWD/tests/update/testdata/relmap-tern go run ./tests/update/compose
MODULE_ROOT=$PWD go run ./tests/update/selfupdate_e2e           # real binary replacement against a local server
go run ./tests/update/ghcli_smoke
```

## License

MIT
