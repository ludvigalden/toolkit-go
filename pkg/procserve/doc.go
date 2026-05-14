// Package procserve keeps warm server processes keyed by an identity.
// Client.Connect dials the identity's unix socket or spawns a detached
// server inheriting a pre-bound listener and lock; Serve runs it.
package procserve
