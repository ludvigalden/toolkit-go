# draft

Code kept for possible future use, outside the module's supported API.
Each directory here is its own Go module, so nothing in it adds to the
dependencies of `github.com/ludvigalden/toolkit-go`.

- `terminal` — an interactive full-screen rendering mode for `output`
  built on Bubble Tea and Lip Gloss. Register it with
  `output.WithRenderer` (or `cli.App.OutputOptions`) via `terminal.Option()`.
