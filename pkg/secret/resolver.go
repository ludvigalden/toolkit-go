package secret

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/secret/internal/ttlcache"
)

// DefaultTTL is the cache lifetime when Options.TTL is zero.
const DefaultTTL = 10 * time.Minute

// ErrUnknown is wrapped by Resolve when no source is registered.
var ErrUnknown = errors.New("no source registered")

// Error reports a secret that did not resolve. It names the secret and
// carries the cause, never a value.
type Error struct {
	Name string
	Err  error
}

// Error renders `secret "name": cause`.
func (e *Error) Error() string { return fmt.Sprintf("secret %q: %v", e.Name, e.Err) }

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Err }

// Options configures a Resolver. Prefix enables reference strings: with
// Prefix "secret://", ResolveRef treats "secret://db" as a reference to
// "db". An empty Prefix disables references. A negative TTL disables
// caching.
type Options struct {
	Sources map[string]Source
	TTL     time.Duration
	Prefix  string
	Now     func() time.Time
}

// Resolver resolves secrets by name, caching each value for the TTL.
// It is safe for concurrent use.
type Resolver struct {
	sources map[string]Source
	prefix  string
	cache   *ttlcache.Cache
}

// NewResolver builds a Resolver; opts.Sources is copied.
func NewResolver(opts Options) *Resolver {
	r := &Resolver{sources: maps.Clone(opts.Sources), prefix: opts.Prefix}
	if r.sources == nil {
		r.sources = map[string]Source{}
	}
	ttl, now := opts.TTL, opts.Now
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if now == nil {
		now = time.Now
	}
	r.cache = ttlcache.New(ttl, now)
	return r
}

// Resolve returns the named secret, fetching it when the cache has no
// live entry. Each call returns a fresh Value the caller may Zero.
// Failures are *Error; unregistered names wrap ErrUnknown.
func (r *Resolver) Resolve(ctx context.Context, name string) (*Value, error) {
	src, ok := r.sources[name]
	if !ok {
		return nil, &Error{Name: name, Err: ErrUnknown}
	}
	if v, ok := r.cache.Get(name); ok {
		return NewValue(v), nil
	}

	s, err := src.Fetch(ctx)
	if err != nil {
		return nil, &Error{Name: name, Err: err}
	}
	r.cache.Put(name, s)
	return NewValue(s), nil
}

// ParseRef reports whether v is a whole-value reference and returns
// the secret name. A value that only contains the prefix is not one.
func (r *Resolver) ParseRef(v string) (string, bool) {
	if r.prefix == "" {
		return "", false
	}
	name, ok := strings.CutPrefix(v, r.prefix)
	if !ok || name == "" || strings.ContainsAny(name, " \t\r\n") {
		return "", false
	}
	return name, true
}

// ResolveRef resolves v when it is a reference; otherwise it returns
// v itself wrapped, with isRef false.
func (r *Resolver) ResolveRef(ctx context.Context, v string) (val *Value, isRef bool, err error) {
	name, ok := r.ParseRef(v)
	if !ok {
		return NewValue(v), false, nil
	}
	val, err = r.Resolve(ctx, name)
	return val, true, err
}

// Purge zeroes and drops every cached value.
func (r *Resolver) Purge() { r.cache.Purge() }
