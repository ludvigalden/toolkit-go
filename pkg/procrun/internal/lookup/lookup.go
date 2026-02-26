// Package lookup resolves executables: PATH lookup with a hit cache,
// executable-file checks, and sibling-binary probing.
package lookup

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// Cache memoizes successful resolutions. Only hits are stored so a tool
// installed mid-run stays discoverable. The zero value is ready to use; a
// nil *Cache caches nothing.
type Cache struct{ m sync.Map }

// Get returns the cached path for key.
func (c *Cache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	v, ok := c.m.Load(key)
	if !ok {
		return "", false
	}
	return v.(string), true //nolint:forcetypeassert // only strings are stored
}

// Put records a resolution.
func (c *Cache) Put(key, path string) {
	if c != nil {
		c.m.Store(key, path)
	}
}

// Path resolves name on PATH to an absolute path, consulting and filling c.
func Path(c *Cache, name string) (string, bool) {
	if p, ok := c.Get(name); ok {
		return p, true
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	if abs, aerr := filepath.Abs(p); aerr == nil {
		p = abs
	}
	c.Put(name, p)
	return p, true
}

// IsExecutable reports a regular file with an execute bit set.
func IsExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

// Sibling probes <dir>/<name>, <dir>/<name>-<GOARCH>, then
// <dir>/<name>-<GOOS>_<GOARCH>; an empty dir means the running
// executable's directory.
func Sibling(dir, name string) (string, bool) {
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", false
		}
		dir = filepath.Dir(exe)
	}
	for _, c := range []string{
		filepath.Join(dir, name),
		filepath.Join(dir, name+"-"+runtime.GOARCH),
		filepath.Join(dir, name+"-"+runtime.GOOS+"_"+runtime.GOARCH),
	} {
		if IsExecutable(c) {
			if abs, err := filepath.Abs(c); err == nil {
				return abs, true
			}
		}
	}
	return "", false
}
