package ipclass

import (
	"net"
	"testing"
)

func TestCanonicalIPv4(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"0x7f000001", "127.0.0.1"},
		{"2130706433", "127.0.0.1"},
		{"127.1", "127.0.0.1"},
		{"127.0.1", "127.0.0.1"},
		{"0177.0.0.01", "127.0.0.1"},
		{"0XA.0", "10.0.0.0"},
		{"4294967295", "255.255.255.255"},
		{"4294967296", ""},
		{"1.16777216", ""},
		{"1.2.65536", ""},
		{"256.0.0.1", ""},
		{"1.2.3.4.5", ""},
		{"08", ""},
		{"0x", ""},
		{"1..2", ""},
		{"example.com", ""},
		{"", ""},
		{"1234567890123456", ""},
	}
	for _, tt := range tests {
		got := CanonicalIPv4(tt.host)
		switch {
		case tt.want == "" && got != nil:
			t.Errorf("CanonicalIPv4(%q) = %v, want nil", tt.host, got)
		case tt.want != "" && !got.Equal(net.ParseIP(tt.want)):
			t.Errorf("CanonicalIPv4(%q) = %v, want %s", tt.host, got, tt.want)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := map[string]string{
		"8.8.8.8":          "",
		"2001:4860::8888":  "",
		"::":               "unspecified",
		"fe80::1":          "link-local",
		"fc00::1":          "private",
		"::7f00:1":         "loopback (embedded in IPv4-compatible IPv6 (::/96))",
		"64:ff9b::808:808": "",
		"64:ff9b:1::a00:1": "",
		"172.16.0.1":       "private",
		"239.255.255.250":  "multicast",
	}
	for in, want := range tests {
		if got := Classify(net.ParseIP(in)); got != want {
			t.Errorf("Classify(%s) = %q, want %q", in, got, want)
		}
	}
}
