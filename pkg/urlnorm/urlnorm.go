// Package urlnorm canonicalizes URLs by removing query parameters that do
// not identify the resource: tracking parameters, session and CSRF
// tokens, and host-specific view parameters.
package urlnorm

import (
	"fmt"
	"net/url"
	"strings"
)

// HostRule drops parameters on matching URLs. A URL matches when its
// hostname equals a domain or is a subdomain of one, or when its path
// contains a hint (for software with no fixed domain).
type HostRule struct {
	Domains   []string
	PathHints []string
	Drop      []string
}

func (r HostRule) matches(host, path string) bool {
	for _, d := range r.Domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	for _, h := range r.PathHints {
		if strings.Contains(path, h) {
			return true
		}
	}
	return false
}

// Options configures a Normalizer. A nil slice selects the defaults; an
// empty non-nil slice disables that category. Parameter names match
// case-sensitively.
type Options struct {
	Tracking  []string
	Session   []string
	HostRules []HostRule
	// StripSession makes Normalize also drop session parameters.
	StripSession bool
}

// Normalizer removes configured query parameters. It is immutable and
// safe for concurrent use.
type Normalizer struct {
	tracking     map[string]struct{}
	session      map[string]struct{}
	rules        []HostRule
	stripSession bool
}

// New returns a Normalizer for opts.
func New(opts Options) *Normalizer {
	tracking, session, rules := opts.Tracking, opts.Session, opts.HostRules
	if tracking == nil {
		tracking = DefaultTrackingParams()
	}
	if session == nil {
		session = DefaultSessionParams()
	}
	if rules == nil {
		rules = DefaultHostRules()
	}
	return &Normalizer{
		tracking:     setOf(tracking),
		session:      setOf(session),
		rules:        append([]HostRule(nil), rules...),
		stripSession: opts.StripSession,
	}
}

// Normalize applies the default Normalizer, which keeps session
// parameters.
func Normalize(rawURL string) (string, error) {
	return New(Options{}).Normalize(rawURL)
}

// Normalize parses rawURL, removes tracking and host-rule parameters
// (and session parameters when configured), and returns the result.
func (n *Normalizer) Normalize(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("urlnorm: parse %q: %w", rawURL, err)
	}
	n.StripTracking(u)
	n.ApplyHostRules(u)
	if n.stripSession {
		n.StripSession(u)
	}
	return u.String(), nil
}

// StripTracking removes tracking parameters from u in place.
func (n *Normalizer) StripTracking(u *url.URL) { dropParams(u, n.tracking) }

// StripSession removes session and CSRF parameters from u in place.
func (n *Normalizer) StripSession(u *url.URL) { dropParams(u, n.session) }

// ApplyHostRules removes the parameters of every matching host rule from
// u in place.
func (n *Normalizer) ApplyHostRules(u *url.URL) {
	host, path := u.Hostname(), u.Path
	for _, r := range n.rules {
		if r.matches(host, path) {
			dropParams(u, setOf(r.Drop))
		}
	}
}

// IsTracking reports whether name is a configured tracking parameter.
func (n *Normalizer) IsTracking(name string) bool {
	_, ok := n.tracking[name]
	return ok
}

// IsSession reports whether name is a configured session parameter.
func (n *Normalizer) IsSession(name string) bool {
	_, ok := n.session[name]
	return ok
}

// dropParams rewrites RawQuery only when something was removed, so
// untouched URLs keep their original encoding and parameter order.
func dropParams(u *url.URL, names map[string]struct{}) {
	if u.RawQuery == "" {
		return
	}
	q := u.Query()
	changed := false
	for name := range q {
		if _, ok := names[name]; ok {
			q.Del(name)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
}

func setOf(names []string) map[string]struct{} {
	s := make(map[string]struct{}, len(names))
	for _, n := range names {
		s[n] = struct{}{}
	}
	return s
}
