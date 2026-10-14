// Package toolname is the tool-wide public API: the names and defaults
// every part of toolname (and any program embedding it) agrees on.
package toolname

// Name is the binary name; it keys the config directory, the cache
// directory and the TOOLNAME_* environment variables.
const Name = "toolname"

// Repo is the GitHub repository releases are published to.
const Repo = "example/toolname"

// Line is the relmap release line the binary belongs to.
const Line = "toolname"
