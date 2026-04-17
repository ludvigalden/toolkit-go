// Package diskfree measures free space and refuses writes that would cross
// a caller-chosen reserve, returning a typed error instead of a late ENOSPC.
package diskfree
