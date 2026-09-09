// Package hostnorm reduces hostnames to their registered domain (eTLD+1)
// using the Public Suffix List, so subdomains of one site share a key for
// per-host state such as rate limits or caches.
package hostnorm

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// ErrEmptyHost is returned when there is no hostname to normalize.
var ErrEmptyHost = errors.New("hostnorm: empty hostname")

// Hostname lowercases hostname, drops a trailing dot and any port, and
// returns its eTLD+1. IP addresses, single-label hosts, and names the
// list cannot split are returned as cleaned.
func Hostname(hostname string) (string, error) {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		return "", ErrEmptyHost
	}
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return host, nil
	}
	if etld1, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return etld1, nil
	}
	return host, nil
}

// URL parses rawURL and returns the eTLD+1 of its host.
func URL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("hostnorm: parse url %q: %w", rawURL, err)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("hostnorm: url %q: %w", rawURL, ErrEmptyHost)
	}
	return Hostname(u.Hostname())
}
