// Package ttlcache holds secret bytes for a TTL and zeroes them on
// expiry or purge, so cached values do not outlive their use.
package ttlcache

import (
	"sync"
	"time"
)

type entry struct {
	val     []byte
	expires time.Time
}

// Cache is a concurrency-safe TTL cache of byte values. Use New.
type Cache struct {
	ttl time.Duration
	now func() time.Time

	mu sync.Mutex
	m  map[string]entry
}

// New returns a cache; ttl <= 0 disables storing.
func New(ttl time.Duration, now func() time.Time) *Cache {
	return &Cache{ttl: ttl, now: now, m: map[string]entry{}}
}

// Get returns a copy of a live value; an expired one is zeroed and
// dropped.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return "", false
	}
	if c.now().Before(e.expires) {
		return string(e.val), true
	}
	clear(e.val)
	delete(c.m, key)
	return "", false
}

// Put stores v until the TTL elapses.
func (c *Cache) Put(key, v string) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = entry{val: []byte(v), expires: c.now().Add(c.ttl)}
}

// Purge zeroes and drops every value.
func (c *Cache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		clear(e.val)
		delete(c.m, k)
	}
}
