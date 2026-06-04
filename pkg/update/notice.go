package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/update/internal/noticecache"
)

// DefaultNoticeTTL is how long a check result is reused before the
// release source is asked again.
const DefaultNoticeTTL = 24 * time.Hour

// Notice prints a one-line "a newer version is available" hint on
// stderr, at most once per TTL, using a cached check result so a tool's
// normal commands never wait on the network more than once a day.
type Notice struct {
	// Tool names the binary; it keys the cache directory and the
	// opt-out variable <TOOL>_NO_UPDATE_NOTIFIER.
	Tool string
	// Current is the running version.
	Current string
	// Checker finds the latest release.
	Checker Checker
	// TTL defaults to DefaultNoticeTTL.
	TTL time.Duration
	// CacheDir overrides <user cache dir>/<tool>.
	CacheDir string
	// UserCacheDir resolves the stdlib user cache directory when
	// CacheDir is empty; nil means os.UserCacheDir.
	UserCacheDir func() (string, error)
	// Out defaults to os.Stderr.
	Out io.Writer
	// Interactive reports whether Out is a terminal; nil means check
	// os.Stderr. Notices are only shown to people, never to pipelines.
	Interactive func() bool
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Getenv reads the opt-out variables; nil means os.Getenv.
	Getenv func(string) string
}

// Enabled reports whether the notice may run: not opted out, not in
// CI, and attached to a terminal.
func (n *Notice) Enabled() bool {
	if n.Tool == "" || n.Checker == nil {
		return false
	}
	getenv := n.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if getenv(noticecache.OptOutVar(n.Tool)) != "" || getenv("CI") != "" {
		return false
	}
	if n.Interactive != nil {
		return n.Interactive()
	}
	return noticecache.IsTerminal(os.Stderr)
}

// Check returns the latest release when newer than Current, cache
// first. found is false when current or unknown; source errors are
// swallowed because a notice must never break a command.
func (n *Notice) Check(ctx context.Context) (latest Release, found bool) {
	now := time.Now
	if n.Now != nil {
		now = n.Now
	}
	ttl := n.TTL
	if ttl <= 0 {
		ttl = DefaultNoticeTTL
	}
	dir := n.CacheDir
	if dir == "" {
		resolve := n.UserCacheDir
		if resolve == nil {
			resolve = os.UserCacheDir
		}
		if d, err := resolve(); err == nil {
			dir = filepath.Join(d, n.Tool)
		}
	}
	path := noticecache.Path(dir, n.Tool)
	c := noticecache.Read(path)
	if !c.Fresh(now(), ttl) {
		rel, err := n.Checker.Latest(ctx)
		if err != nil {
			return Release{}, false
		}
		c = noticecache.Entry{CheckedAt: now(), Latest: rel.Version, URL: rel.URL}
		noticecache.Write(path, c)
	}
	if !Newer(c.Latest, n.Current) {
		return Release{}, false
	}
	return Release{Version: c.Latest, Tag: "v" + c.Latest, URL: c.URL}, true
}

// Show prints the notice line when enabled and a newer release exists.
// It reports whether a line was printed.
func (n *Notice) Show(ctx context.Context) bool {
	if !n.Enabled() {
		return false
	}
	rel, ok := n.Check(ctx)
	if !ok {
		return false
	}
	out := n.Out
	if out == nil {
		out = os.Stderr
	}
	_, _ = fmt.Fprintf(out, "%s: version %s is available (running %s); run `%s update`\n",
		n.Tool, rel.Version, n.Current, n.Tool)
	return true
}

// IsUnavailable reports whether err means a capability is absent.
func IsUnavailable(err error) bool { return errors.Is(err, ErrUnavailable) }
