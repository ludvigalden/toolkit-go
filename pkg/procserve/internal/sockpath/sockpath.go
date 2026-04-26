// Package sockpath derives an identity's socket, temporary bind, and
// lock paths so they fit the unix socket path limit, and guards the
// shared fallback directory.
package sockpath

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// maxTempBind is the usable sun_path length on the smallest supported
// platform (macOS: 104 including the NUL); the temporary bind name, the
// longest path bound, must fit.
const maxTempBind = 103

// File suffixes of one identity's neighbourhood.
const (
	SockSuffix = ".sock"
	TempSuffix = ".tmp"
	LockSuffix = ".lock"
)

// Paths is one identity's socket neighbourhood.
type Paths struct {
	Socket, Temp, Lock string
}

// Hash returns the hex SHA-256 of an identity key.
func Hash(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

// name is the 26-character base32 of the identity hash's first 16 bytes:
// short enough for deep directories, collision-free in practice.
func name(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:16])
}

// In returns the neighbourhood under dir, or false when the temporary
// bind name would not fit in sun_path.
func In(dir, identity string) (Paths, bool) {
	sock := filepath.Join(dir, name(identity)+SockSuffix)
	if len(sock)+len(TempSuffix) > maxTempBind {
		return Paths{}, false
	}
	return Paths{Socket: sock, Temp: sock + TempSuffix, Lock: sock + LockSuffix}, true
}

// Resolve picks dir when its paths fit, else fallback, creating the
// chosen directory 0700; fallback must also be private to this user.
func Resolve(dir, fallback, identity string) (Paths, error) {
	if p, ok := In(dir, identity); ok {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("procserve: socket dir: %w", err)
		}
		return p, nil
	}
	if err := EnsurePrivateDir(fallback); err != nil {
		return Paths{}, err
	}
	p, ok := In(fallback, identity)
	if !ok {
		return Paths{}, fmt.Errorf("procserve: fallback dir %s is too long for a socket path", fallback)
	}
	return p, nil
}

// DefaultFallback is the short per-user directory under /tmp.
func DefaultFallback() string {
	return "/tmp/procserve-" + strconv.Itoa(os.Getuid())
}

// EnsurePrivateDir creates dir 0700 and verifies that it is a real
// directory owned by this user and closed to others: the fallback lives
// in a shared location where another user could pre-create it.
func EnsurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("procserve: fallback dir: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("procserve: fallback dir: %w", err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || int(st.Uid) != os.Getuid() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("procserve: fallback dir %s must be a 0700 directory owned by this user", dir)
	}
	return nil
}

// Inode returns a path's inode number.
func Inode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err //nolint:wrapcheck // callers test fs.ErrNotExist
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("procserve: no inode for %s", path)
	}
	return st.Ino, nil
}
