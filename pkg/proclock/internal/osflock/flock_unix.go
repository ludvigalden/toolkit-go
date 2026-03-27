//go:build linux || darwin

package osflock

import (
	"errors"

	"golang.org/x/sys/unix"
)

// TryLockEx takes a non-blocking exclusive advisory lock on fd. It returns
// nil on success and a non-nil error (unix.EWOULDBLOCK / unix.EAGAIN) when
// the lock is already held by another open file description.
func TryLockEx(fd uintptr) error {
	return unix.Flock(int(fd), unix.LOCK_EX|unix.LOCK_NB)
}

// TryLockSh takes a non-blocking shared advisory lock on fd, used by
// readers to probe whether an exclusive holder is present.
func TryLockSh(fd uintptr) error {
	return unix.Flock(int(fd), unix.LOCK_SH|unix.LOCK_NB)
}

// Contended reports whether err is the "lock is held" error returned by a
// non-blocking flock attempt.
func Contended(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
