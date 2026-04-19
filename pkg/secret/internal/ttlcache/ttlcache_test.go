package ttlcache

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	now := time.Unix(0, 0)
	c := New(time.Minute, func() time.Time { return now })
	c.Put("k", "v")
	if v, ok := c.Get("k"); !ok || v != "v" {
		t.Fatalf("Get = %q %v", v, ok)
	}
	now = now.Add(2 * time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expired value returned")
	}
	c.Put("k", "v")
	c.Purge()
	if _, ok := c.Get("k"); ok {
		t.Fatal("purged value returned")
	}
	off := New(-1, time.Now)
	off.Put("k", "v")
	if _, ok := off.Get("k"); ok {
		t.Fatal("disabled cache stored")
	}
}
