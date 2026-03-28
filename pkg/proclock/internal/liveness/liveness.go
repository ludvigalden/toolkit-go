// Package liveness decides whether a recorded lock owner still runs:
// PID probe, per-boot identity, and an executable check against PID
// reuse where the platform exposes one.
package liveness

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// Alive reports a PID that is running in the boot identified by boot
// ("" skips the boot check).
func Alive(pid int, boot string) bool {
	if pid <= 0 {
		return false
	}
	if boot != "" {
		if cur := BootID(); cur != "" && cur != boot {
			return false
		}
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// AliveAndOurs adds an executable check against binary ("" skips it),
// defeating PID reuse in the flock-free reclaim window.
func AliveAndOurs(pid int, boot, binary string) bool {
	if !Alive(pid, boot) {
		return false
	}
	if binary == "" {
		return true
	}
	exe, ok := exePath(pid)
	return !ok || samePath(exe, binary)
}

// BootID returns the Linux per-boot identity, "" elsewhere.
func BootID() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Nonce mints a random 128-bit hex token identifying one acquisition.
func Nonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("proclock: nonce: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// exePath resolves a PID's executable (Linux only).
func exePath(pid int) (string, bool) {
	if runtime.GOOS != "linux" {
		return "", false
	}
	link, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	return link, err == nil && link != ""
}

// samePath compares two paths after resolving symlinks.
func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = a
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = b
	}
	return ra == rb
}
