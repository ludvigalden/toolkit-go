package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
)

type stubResolver struct {
	addrs map[string][]string
	err   error
}

func (s stubResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if s.err != nil {
		return nil, s.err
	}
	var out []net.IPAddr
	for _, a := range s.addrs[host] {
		out = append(out, net.IPAddr{IP: net.ParseIP(a)})
	}
	return out, nil
}

func TestValidateURL(t *testing.T) {
	v := New(stubResolver{addrs: map[string][]string{
		"public.test":   {"93.184.216.34"},
		"internal.test": {"93.184.216.34", "10.0.0.5"},
		"empty.test":    nil,
	}})
	tests := []struct {
		raw    string
		reject string
	}{
		{"https://public.test/path", ""},
		{"http://93.184.216.34:8080/", ""},
		{"https://[2606:4700::1111]/", ""},
		{"ftp://public.test/", "scheme"},
		{"https:///nohost", "host is empty"},
		{"http://127.0.0.1/", "loopback"},
		{"http://[::1]/", "loopback"},
		{"http://10.1.2.3/", "private"},
		{"http://169.254.169.254/latest/meta-data", "link-local"},
		{"http://0.0.0.0/", "unspecified"},
		{"http://0.1.2.3/", "this-network"},
		{"http://240.0.0.1/", "reserved"},
		{"http://255.255.255.255/", "reserved"},
		{"http://224.0.0.1/", "multicast"},
		{"http://[ff01::1]/", "interface-local multicast"},
		{"http://[::ffff:127.0.0.1]/", "loopback"},
		{"http://[::10.0.0.1]/", "private (embedded in IPv4-compatible"},
		{"http://[64:ff9b::a9fe:a9fe]/", "link-local (embedded in NAT64"},
		{"http://[64:ff9b::5db8:d822]/", ""},
		{"http://0x7f000001/", "loopback"},
		{"http://2130706433/", "loopback"},
		{"http://127.1/", "loopback"},
		{"http://0177.0.0.1/", "loopback"},
		{"http://10.0x10203/", "private"},
		{"https://internal.test/", "resolves to 10.0.0.5 which is private"},
		{"https://empty.test/", "resolved to no addresses"},
		{"http://%zz/", "parse URL"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			err := v.ValidateURL(context.Background(), tt.raw)
			if tt.reject == "" {
				if err != nil {
					t.Fatalf("ValidateURL = %v, want nil", err)
				}
				return
			}
			if !IsValidationError(err) || !strings.Contains(err.Error(), tt.reject) {
				t.Fatalf("ValidateURL = %v, want rejection containing %q", err, tt.reject)
			}
			if !errors.Is(err, errkind.InvalidInput) {
				t.Fatalf("kind of %v is not InvalidInput", err)
			}
		})
	}
}

func TestResolveFailureKinds(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", Name: "x.test", IsNotFound: true}
	err := New(stubResolver{err: notFound}).ValidateURL(context.Background(), "https://x.test/")
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) || !errors.Is(err, errkind.InvalidInput) {
		t.Fatalf("not-found error = %v, want InvalidInput wrapping *net.DNSError", err)
	}
	boom := errors.New("resolver down")
	err = New(stubResolver{err: boom}).ValidateURL(context.Background(), "https://x.test/")
	if !errors.Is(err, boom) || !errors.Is(err, errkind.Unavailable) || !IsValidationError(err) {
		t.Fatalf("resolver failure = %v, want Unavailable wrapping cause", err)
	}
}

func TestCheckIP(t *testing.T) {
	if err := CheckIP(net.ParseIP("1.1.1.1")); err != nil {
		t.Fatalf("CheckIP(public) = %v", err)
	}
	if err := CheckIP(net.ParseIP("192.168.1.1")); !IsValidationError(err) {
		t.Fatalf("CheckIP(private) = %v", err)
	}
}

// recordingDialer records the address it was asked for and connects to
// target instead, so the flow can be exercised against a local
// listener with a public-classified validated address.
type recordingDialer struct {
	target string
	dialed string
}

func (d *recordingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d.dialed = addr
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, d.target)
}

func TestDialContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback listener: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	v := New(stubResolver{addrs: map[string][]string{
		"public.test":  {"93.184.216.34"},
		"private.test": {"10.0.0.5"},
	}})
	d := &recordingDialer{}
	v.Dialer = d
	ctx := context.Background()

	// Denied before any dial: loopback literal and a host that
	// resolves to a private address.
	for _, addr := range []string{
		net.JoinHostPort("127.0.0.1", fmt.Sprint(port)),
		net.JoinHostPort("private.test", fmt.Sprint(port)),
	} {
		if _, err := v.DialContext(ctx, "tcp", addr); !IsValidationError(err) {
			t.Fatalf("DialContext(%s) = %v, want a validation error", addr, err)
		}
	}
	if d.dialed != "" {
		t.Fatalf("denied address %q was dialed", d.dialed)
	}

	// Success: the connection lands on the local listener while the
	// dialer saw the validated public address, not a re-resolution.
	d.target = ln.Addr().String()
	conn, err := v.DialContext(ctx, "tcp", net.JoinHostPort("public.test", fmt.Sprint(port)))
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer conn.Close()
	if want := net.JoinHostPort("93.184.216.34", fmt.Sprint(port)); d.dialed != want {
		t.Fatalf("dialer was asked for %q, want %q", d.dialed, want)
	}
}
