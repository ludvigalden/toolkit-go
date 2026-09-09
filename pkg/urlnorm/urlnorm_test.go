package urlnorm

import (
	"net/url"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://example.com/a?utm_source=x&id=1", "https://example.com/a?id=1"},
		{"https://example.com/a?b=2&a=1", "https://example.com/a?b=2&a=1"},
		{"https://example.com/a?fbclid=1&gclid=2", "https://example.com/a"},
		{"https://www.youtube.com/watch?v=abc&t=42&list=L", "https://www.youtube.com/watch?v=abc"},
		{"https://old.reddit.com/r/go?sort=new&x=1", "https://old.reddit.com/r/go?x=1"},
		{"https://notreddit.com/?sort=new", "https://notreddit.com/?sort=new"},
		{"https://forum.test/viewtopic.php?t=5&sid=abc", "https://forum.test/viewtopic.php?t=5"},
		{"https://example.com/?PHPSESSID=1&q=x", "https://example.com/?PHPSESSID=1&q=x"},
		{"https://example.com/?UTM_SOURCE=x", "https://example.com/?UTM_SOURCE=x"},
	}
	for _, tt := range tests {
		got, err := Normalize(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	if _, err := Normalize("http://%zz"); err == nil {
		t.Error("Normalize accepted an unparsable URL")
	}
}

func TestOptions(t *testing.T) {
	n := New(Options{StripSession: true})
	got, err := n.Normalize("https://example.com/?PHPSESSID=1&csrf_token=2&q=x&utm_id=3")
	if err != nil || got != "https://example.com/?q=x" {
		t.Fatalf("StripSession Normalize = %q, %v", got, err)
	}
	none := New(Options{Tracking: []string{}, Session: []string{}, HostRules: []HostRule{}})
	raw := "https://youtu.be/x?utm_source=a&t=1"
	if got, _ := none.Normalize(raw); got != raw {
		t.Fatalf("empty options changed URL: %q", got)
	}
	custom := New(Options{Tracking: []string{"src"}, HostRules: []HostRule{{Domains: []string{"a.test"}, Drop: []string{"v"}}}})
	if got, _ := custom.Normalize("https://b.a.test/?src=1&v=2&utm_id=3"); got != "https://b.a.test/?utm_id=3" {
		t.Fatalf("custom Normalize = %q", got)
	}
}

func TestPredicatesAndInPlace(t *testing.T) {
	n := New(Options{})
	if !n.IsTracking("utm_source") || n.IsTracking("id") || !n.IsSession("SID") || n.IsSession("sid") {
		t.Fatal("predicate mismatch")
	}
	u, _ := url.Parse("https://example.com/?SID=1&k=v")
	n.StripSession(u)
	if u.RawQuery != "k=v" {
		t.Fatalf("StripSession left %q", u.RawQuery)
	}
}

func TestDefaultsAreFresh(t *testing.T) {
	a := DefaultTrackingParams()
	a[0] = "mutated"
	if DefaultTrackingParams()[0] == "mutated" {
		t.Fatal("DefaultTrackingParams shares its backing array")
	}
	if len(DefaultSessionParams()) == 0 || len(DefaultHostRules()) == 0 {
		t.Fatal("empty defaults")
	}
}
