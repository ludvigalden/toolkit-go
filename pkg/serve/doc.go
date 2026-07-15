// Package serve runs HTTP servers with graceful shutdown: /healthz and
// /readyz endpoints are wrapped around the caller's handler, and the
// server stops when its context is canceled, serving fails, or the
// executable is replaced on disk.
package serve
