package hostnorm

import (
	"errors"
	"testing"
)

func TestHostname(t *testing.T) {
	tests := []struct{ in, want string }{
		{"www.example.com", "example.com"},
		{"a.b.example.co.uk", "example.co.uk"},
		{"EXAMPLE.COM.", "example.com"},
		{"  api.example.org:8443 ", "example.org"},
		{"example.com", "example.com"},
		{"localhost", "localhost"},
		{"192.168.1.10", "192.168.1.10"},
		{"[::1]:80", "::1"},
		{"2001:db8::1", "2001:db8::1"},
		{"co.uk", "co.uk"},
		{"foo.bar.unknowntld", "bar.unknowntld"},
	}
	for _, tt := range tests {
		got, err := Hostname(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("Hostname(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "  ", "."} {
		if _, err := Hostname(in); !errors.Is(err, ErrEmptyHost) {
			t.Errorf("Hostname(%q) err = %v, want ErrEmptyHost", in, err)
		}
	}
}

func TestURL(t *testing.T) {
	got, err := URL("https://docs.example.com:443/path?q=1")
	if err != nil || got != "example.com" {
		t.Fatalf("URL = %q, %v", got, err)
	}
	if _, err := URL("/relative/path"); !errors.Is(err, ErrEmptyHost) {
		t.Fatalf("URL(no host) err = %v", err)
	}
	if _, err := URL("http://%zz"); err == nil {
		t.Fatal("URL accepted an unparsable URL")
	}
}
