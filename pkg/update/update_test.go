package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
	"github.com/ludvigalden/toolkit-go/pkg/update/internal/noticecache"
)

func TestUnavailableIsClassified(t *testing.T) {
	err := fmt.Errorf("ghcli: %w", ErrUnavailable)
	if !IsUnavailable(err) || !errors.Is(err, errkind.Unavailable) {
		t.Fatal("wrapped ErrUnavailable lost its identity or kind")
	}
	if got := errkind.ExitCode(err); got != exitcode.Unavailable {
		t.Errorf("exit code = %d", got)
	}
}

func TestNoticeOptOutUsesGetenv(t *testing.T) {
	n := &Notice{Tool: "my-tool", Checker: stubChecker{}, Interactive: func() bool { return true }}
	n.Getenv = func(k string) string { return map[string]string{"MY_TOOL_NO_UPDATE_NOTIFIER": "1"}[k] }
	if n.Enabled() {
		t.Error("opt-out variable ignored")
	}
	n.Getenv = func(string) string { return "" }
	if !n.Enabled() {
		t.Error("notice disabled without opt-out")
	}
}

func TestNewerAndVersionFromTag(t *testing.T) {
	if !Newer("v1.2.0", "1.1.9") || Newer("1.0.0", "v1.0.0") {
		t.Error("Newer")
	}
	if VersionFromTag("v2.0.1") != "2.0.1" || VersionFromTag("vnext") != "vnext" {
		t.Error("VersionFromTag")
	}
}

type stubChecker struct{}

func (stubChecker) Latest(context.Context) (Release, error) { return Release{}, ErrUnavailable }

// countingChecker records how often Latest ran.
type countingChecker struct{ calls int }

func (c *countingChecker) Latest(context.Context) (Release, error) {
	c.calls++
	return Release{Version: "1.1.0"}, nil
}

// The injected user-cache-dir resolver decides where the notice cache
// lives, and a fresh cached entry keeps the release source idle.
func TestNoticeCheckUsesInjectedCacheDir(t *testing.T) {
	cacheRoot := t.TempDir()
	checker := &countingChecker{}
	n := &Notice{
		Tool:        "my-tool",
		Current:     "1.0.0",
		Checker:     checker,
		Interactive: func() bool { return true },
		Getenv:      func(string) string { return "" },
		UserCacheDir: func() (string, error) {
			return cacheRoot, nil
		},
	}
	rel, found := n.Check(context.Background())
	if !found || rel.Version != "1.1.0" || checker.calls != 1 {
		t.Fatalf("first check: %+v found=%v calls=%d", rel, found, checker.calls)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "my-tool", noticecache.FileName)); err != nil {
		t.Fatalf("cache file: %v", err)
	}
	if _, found := n.Check(context.Background()); !found || checker.calls != 1 {
		t.Fatalf("second check ran the release source again (calls=%d)", checker.calls)
	}
}

// An unresolvable cache directory means no caching, never a failure.
func TestNoticeCheckWithoutCacheDir(t *testing.T) {
	n := &Notice{
		Tool:        "my-tool",
		Current:     "1.0.0",
		Checker:     &countingChecker{},
		Interactive: func() bool { return true },
		Getenv:      func(string) string { return "" },
		UserCacheDir: func() (string, error) {
			return "", errors.New("no cache dir")
		},
	}
	if _, found := n.Check(context.Background()); !found {
		t.Fatal("check failed because the cache directory was missing")
	}
}
