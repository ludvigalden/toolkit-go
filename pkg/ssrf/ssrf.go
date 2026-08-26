package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/ssrf/internal/ipclass"
)

// Resolver looks up the addresses of a hostname. *net.Resolver satisfies
// it.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Dialer makes the outgoing connection. *net.Dialer satisfies it.
type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// ValidationError explains why a URL was rejected. Cause holds the
// underlying error (such as a *net.DNSError) when there is one.
type ValidationError struct {
	Reason string
	Cause  error
}

// Error implements error.
func (e *ValidationError) Error() string { return "ssrf: " + e.Reason }

// Unwrap returns the underlying cause, or nil for a policy rejection.
func (e *ValidationError) Unwrap() error { return e.Cause }

// IsValidationError reports whether err wraps a *ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// Validator checks URLs against the public-address policy.
type Validator struct {
	// Resolver resolves hostnames; nil uses net.DefaultResolver.
	Resolver Resolver
	// Dialer makes the connection for DialContext; nil uses a zero
	// net.Dialer.
	Dialer Dialer
}

// New returns a Validator using r, or net.DefaultResolver when r is nil.
func New(r Resolver) *Validator {
	return &Validator{Resolver: r}
}

// ValidateURL validates raw with net.DefaultResolver.
func ValidateURL(ctx context.Context, raw string) error {
	return (&Validator{}).ValidateURL(ctx, raw)
}

// CheckIP reports a *ValidationError when ip is not a public address.
func CheckIP(ip net.IP) error {
	if reason := ipclass.Classify(ip); reason != "" {
		return reject(errkind.InvalidInput, reason, nil)
	}
	return nil
}

// ValidateURL rejects non-http(s) URLs and hosts that are, or resolve to,
// non-public addresses (*ValidationError: InvalidInput, or Unavailable
// when resolution failed). A later dial re-resolves DNS and can be
// rebound to a private address, so fetchers dial through DialContext.
func (v *Validator) ValidateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return reject(errkind.InvalidInput, fmt.Sprintf("parse URL: %v", err), err)
	}
	if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
		return reject(errkind.InvalidInput, fmt.Sprintf("scheme %q is not allowed (only http, https)", u.Scheme), nil)
	}
	host := u.Hostname()
	if host == "" {
		return reject(errkind.InvalidInput, "host is empty", nil)
	}
	if addr, perr := netip.ParseAddr(host); perr == nil {
		return CheckIP(net.IP(addr.AsSlice()))
	}
	// Dial stacks accept inet_aton spellings such as 0x7f000001 or 127.1
	// as literals, so classify them locally instead of asking DNS.
	if v4 := ipclass.CanonicalIPv4(host); v4 != nil {
		return CheckIP(v4)
	}
	return v.checkResolved(ctx, host)
}

// DialContext resolves addr, rejects it unless every resolved address is
// public, and dials one of those validated addresses, so the connection
// goes where the check looked. Fetchers use it instead of ValidateURL plus
// a plain dial, whose own DNS lookup opens a rebinding window.
func (v *Validator) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, reject(errkind.InvalidInput, fmt.Sprintf("split address: %v", err), err)
	}
	var ips []net.IP
	if ip, perr := netip.ParseAddr(host); perr == nil {
		lit := net.IP(ip.AsSlice())
		if cerr := CheckIP(lit); cerr != nil {
			return nil, cerr
		}
		ips = []net.IP{lit}
	} else if v4 := ipclass.CanonicalIPv4(host); v4 != nil {
		// Dial stacks accept inet_aton spellings such as 0x7f000001
		// as literals, so classify them locally instead of asking DNS.
		if cerr := CheckIP(v4); cerr != nil {
			return nil, cerr
		}
		ips = []net.IP{v4}
	} else {
		var rerr error
		if ips, rerr = v.validatedIPs(ctx, host); rerr != nil {
			return nil, rerr
		}
	}
	dialer := Dialer(&net.Dialer{})
	if v.Dialer != nil {
		dialer = v.Dialer
	}
	var firstErr error
	for _, ip := range ips {
		conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = derr
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("dial %s: %w", addr, firstErr)
}

func (v *Validator) checkResolved(ctx context.Context, host string) error {
	_, err := v.validatedIPs(ctx, host)
	return err
}

// validatedIPs resolves host and returns its addresses, rejecting the
// host unless every one of them is public.
func (v *Validator) validatedIPs(ctx context.Context, host string) ([]net.IP, error) {
	var r Resolver = net.DefaultResolver
	if v.Resolver != nil {
		r = v.Resolver
	}
	addrs, err := r.LookupIPAddr(ctx, host)
	if err != nil {
		kind := errkind.Unavailable
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			kind = errkind.InvalidInput
		}
		return nil, reject(kind, fmt.Sprintf("resolve host %q: %v", host, err), err)
	}
	if len(addrs) == 0 {
		return nil, reject(errkind.InvalidInput, fmt.Sprintf("host %q resolved to no addresses", host), nil)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if reason := ipclass.Classify(a.IP); reason != "" {
			return nil, reject(errkind.InvalidInput, fmt.Sprintf("host %q resolves to %s which is %s", host, a.IP, reason), nil)
		}
		ips = append(ips, a.IP)
	}
	return ips, nil
}

func reject(kind errkind.Kind, reason string, cause error) error {
	return errkind.Wrap(kind, &ValidationError{Reason: reason, Cause: cause}, "")
}
