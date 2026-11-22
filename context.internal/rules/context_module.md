---
paths:
  - context/*.yaml
  - context/profiles/*/*.yaml
  - context/profiles/*/packages/*/*.md
  - context/profiles/*/packages/*/reference/*.md
---
`context/` is the committed ctxmod module `toolkit_go` that consumers
fetch. It stays in the public vocabulary and never refers to local
overlays or development-only context. Every Go tree under
`context/examples/` and `context/profiles/*/examples/` is its own
module with a `replace github.com/ludvigalden/toolkit-go => <relative
root>` directive, and must pass `go build ./...` and `go vet ./...`
from its own directory (`go run ./tests/examples/build` checks them
all). Re-run `ctxmod vet --root .` unpiped after each edit and record
its literal exit code.
