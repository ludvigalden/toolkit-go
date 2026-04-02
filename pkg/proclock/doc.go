// Package proclock is a cross-process lock: an advisory flock on a file
// holding JSON owner metadata, so contenders can name or connect to the
// running owner. The kernel drops the flock when the owner dies, so a
// crash never wedges the lock.
package proclock
