---
description: >-
  Building a Go command-line tool on toolkit-go: the canonical layout,
  the package map, and release awareness through an embedded relmap
  changelog export.
binds_to_depth: main...
---

# Building tools on toolkit-go

toolkit-go (`github.com/ludvigalden/toolkit-go`) is a library module;
every public package lives under `pkg/` and is imported as
`github.com/ludvigalden/toolkit-go/pkg/<name>`. Import only what a
tool needs: each package stands alone, and the update stack is built
from small interfaces a tool composes rather than inherits.

A tool repository follows one layout: `tools/<tool>/cmd/main.go` hands
`os.Args` to `tools/<tool>/internal/app`, the tool's importable API
lives under `pkg/go/<tool>/`, code shared by several of the
repository's tools under `pkg/go/common/<name>/`, and implementation
detail under an `internal/` directory beside the API it serves. The
complete, buildable tool in `examples/go_tool_toolname/` is the
reference; copy it to start.

Changelogs are authored with relmap (`map/` and `changelog/`), exported
with `relmap export --target ndjson,index,metadata`, and embedded with
`go:embed`; `update/relmapx` reads the export. Stdout carries data
payloads only; diagnostics, notices and progress go to stderr.

Read on demand:

- which package covers which concern — `reference/packages.md`
- release awareness: embedding the export, version, changelog,
  self-update, notices, and the release workflow — `reference/release_awareness.md`
- the usage examples and what each demonstrates — `reference/examples.md`
