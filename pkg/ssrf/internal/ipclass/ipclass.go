// Package ipclass classifies IP addresses as public or non-public and
// parses the nonstandard IPv4 spellings dial stacks accept.
package ipclass

import (
	"net"
	"strings"
)

// Classify returns why ip is not public, or "" when it is.
func Classify(ip net.IP) string {
	switch {
	case ip.IsLoopback():
		return "loopback"
	case ip.IsPrivate():
		return "private"
	case ip.IsLinkLocalUnicast():
		return "link-local"
	case ip.IsUnspecified():
		return "unspecified"
	case ip.IsInterfaceLocalMulticast():
		return "interface-local multicast"
	case ip.IsMulticast():
		return "multicast"
	}
	if v4 := ip.To4(); v4 != nil {
		// Several platforms route 0.0.0.0/8 to loopback; 240.0.0.0/4
		// (with 255.255.255.255) is reserved. No public host lives in either.
		if v4[0] == 0 {
			return "this-network (0.0.0.0/8)"
		}
		if v4[0]&0xF0 == 0xF0 {
			return "reserved (240.0.0.0/4)"
		}
		return ""
	}
	if embedded, kind := embeddedIPv4(ip); kind != "" {
		if reason := Classify(embedded); reason != "" {
			return reason + " (embedded in " + kind + ")"
		}
	}
	return ""
}

// embeddedIPv4 extracts the IPv4 inside an IPv4-compatible (::/96) or
// NAT64 (64:ff9b::/96) address; IPv4-mapped forms are handled by To4.
func embeddedIPv4(ip net.IP) (net.IP, string) {
	b := ip.To16()
	if len(b) != net.IPv6len {
		return nil, ""
	}
	tail := net.IPv4(b[12], b[13], b[14], b[15])
	if allZero(b[:12]) {
		return tail, "IPv4-compatible IPv6 (::/96)"
	}
	if b[0] == 0x00 && b[1] == 0x64 && b[2] == 0xff && b[3] == 0x9b && allZero(b[4:12]) {
		return tail, "NAT64 (64:ff9b::/96)"
	}
	return nil, ""
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// CanonicalIPv4 parses host under the inet_aton grammar (1-4 parts, each
// hex, octal, or decimal) and returns the address, or nil when host is
// not numeric in that grammar.
func CanonicalIPv4(host string) net.IP {
	if host == "" || len(host) > len("255.255.255.255") {
		return nil
	}
	parts := strings.Split(host, ".")
	if len(parts) > 4 {
		return nil
	}
	vals := make([]uint64, len(parts))
	for i, p := range parts {
		v, ok := parseAtonPart(p)
		if !ok {
			return nil
		}
		vals[i] = v
	}
	// The last part fills every byte the earlier parts leave.
	var addr uint64
	for i, v := range vals[:len(vals)-1] {
		if v > 0xFF {
			return nil
		}
		addr |= v << (24 - 8*i)
	}
	last := vals[len(vals)-1]
	if last >= 1<<(32-8*(len(vals)-1)) {
		return nil
	}
	addr |= last
	return net.IPv4(byte(addr>>24), byte(addr>>16), byte(addr>>8), byte(addr))
}

// parseAtonPart parses one part: 0x-prefixed hex, 0-prefixed octal, or
// decimal. The 15-byte host cap keeps every value far from overflow.
func parseAtonPart(p string) (uint64, bool) {
	base, digits := uint64(10), p
	switch {
	case len(p) > 2 && p[0] == '0' && (p[1] == 'x' || p[1] == 'X'):
		base, digits = 16, p[2:]
	case len(p) > 1 && p[0] == '0':
		base, digits = 8, p[1:]
	}
	if digits == "" {
		return 0, false
	}
	var val uint64
	for _, c := range digits {
		var d uint64
		switch {
		case c >= '0' && c <= '9':
			d = uint64(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint64(c-'A') + 10
		default:
			return 0, false
		}
		if d >= base {
			return 0, false
		}
		val = val*base + d
	}
	return val, true
}
