package procserve

import (
	"errors"

	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/sockpath"
)

// resolvePaths picks Dir when its socket paths fit, else the short
// fallback dir, creating the chosen directory 0700.
func (c *Client) resolvePaths(identity string) (sockpath.Paths, error) {
	if c.Dir == "" {
		return sockpath.Paths{}, errors.New("procserve: Client.Dir is required")
	}
	return sockpath.Resolve(c.Dir, c.fallbackDir(), identity) //nolint:wrapcheck // errors carry the procserve prefix
}

// fallbackDir is the short directory used when Dir is too deep.
func (c *Client) fallbackDir() string {
	if c.FallbackDir != "" {
		return c.FallbackDir
	}
	return sockpath.DefaultFallback()
}

// dirs lists the directories that may hold this client's servers.
func (c *Client) dirs() []string {
	if fb := c.fallbackDir(); fb != c.Dir {
		return []string{c.Dir, fb}
	}
	return []string{c.Dir}
}
