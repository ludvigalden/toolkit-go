# process_runner

`runall` runs commands one after another in a directory, streaming each
command's output and reporting its exit code, duration, and peak memory:

- `common/procrun` — a `RunContext` bound to the directory, per-step
  `Timeout`, streaming stdout with `Start` + `StreamLines`, typed
  `ExitError` / `TimeoutError`, and a custom `Scoper` (`NiceScoper`) that
  runs steps under `nice` when they name the `background` profile.
- `common/proclock` — one `runall` per directory (locks live under the
  user cache dir): a second instance fails fast and names the owner's PID.
- `procrun.ReadStats` — RSS sampling of the running step.

Each positional argument is one command, split on whitespace (no shell
quoting; wrap shell syntax in a script).

```sh
go run ./tools/runall/cmd "go version" "ls"
go run ./tools/runall/cmd --background --timeout 2s "sleep 5"
```
