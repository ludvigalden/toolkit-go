# repo_walker

`census` counts a repository's files by extension with `common/fsenum`:

- `fsenum.Walk` — a gitignore-honoring walk (fd when installed, a
  built-in walker otherwise) yielding metadata-only entries.
- `fsenum.Default()` — the categorized convenience set (version control,
  dependencies, build outputs, caches, coverage, editor and OS files,
  binaries, archives, databases); `Union` adds the tool's own `.census/`
  cache directory and any `--skip` names.

```sh
go run ./tools/census/cmd .
go run ./tools/census/cmd --json --skip fixtures /path/to/repo
```
