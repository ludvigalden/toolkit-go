// Package textfmt is a repository-common package: helpers several of
// the repository's tools share. Its implementation lives in internal/.
package textfmt

import "example.com/toolname/pkg/go/common/textfmt/internal/wrap"

// Wrap breaks s into lines of at most width runes at word boundaries.
func Wrap(s string, width int) []string { return wrap.Words(s, width) }
