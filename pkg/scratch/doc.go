// Package scratch builds hermetic scratch environments for CLI tools:
// a per-run fenced HOME, TMPDIR, XDG homes, working directory, and
// fixtures in one root removed at run end.
// Hygiene, not a security sandbox.
package scratch
