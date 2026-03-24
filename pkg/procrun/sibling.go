package procrun

import "github.com/ludvigalden/toolkit-go/pkg/procrun/internal/lookup"

// SiblingResolver resolves a tool next to a base executable, probing
// <dir>/<name>, <dir>/<name>-<GOARCH>, then <dir>/<name>-<GOOS>_<GOARCH>.
// An empty BaseDir means the running executable's directory.
type SiblingResolver struct {
	BaseDir string
}

// Resolve implements Resolver.
func (s SiblingResolver) Resolve(name string) (string, bool) {
	return lookup.Sibling(s.BaseDir, name)
}
