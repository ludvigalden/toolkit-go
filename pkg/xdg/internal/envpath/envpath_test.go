package envpath

import (
	"errors"
	"slices"
	"testing"
)

func TestHomeAndList(t *testing.T) {
	env := map[string]string{"HOME": "/h", "REL": "rel", "LIST": "rel:/a/:/b"}
	get := func(k string) string { return env[k] }
	if got, err := Home(get, "REL", ".config"); err != nil || got != "/h/.config" {
		t.Fatalf("Home = %q %v", got, err)
	}
	if _, err := Home(func(string) string { return "" }, "X", "y"); !errors.Is(err, ErrNoHome) {
		t.Fatalf("missing HOME = %v", err)
	}
	if got := List(get, "LIST", "/d"); !slices.Equal(got, []string{"/a", "/b"}) {
		t.Fatalf("List = %v", got)
	}
	if got := List(get, "NONE", "/d"); !slices.Equal(got, []string{"/d"}) {
		t.Fatalf("List default = %v", got)
	}
}
