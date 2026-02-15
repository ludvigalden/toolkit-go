// Package kongx holds the Kong plumbing behind cli: parsing without
// letting Kong exit the process, honoring a quiet request before flags
// apply, and mapping explicitly given flags to config keys.
package kongx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
)

// ExitSignal carries Kong's exit request (after --help) out of Parse
// without terminating the process; install it with kong.Exit.
type ExitSignal int

// Exit is the kong.Exit hook that raises ExitSignal.
func Exit(code int) { panic(ExitSignal(code)) }

// Parse parses args. done reports that the caller should exit with
// code: after --help, or with usageCode for bad arguments (printed to
// the parser's stderr unless a quiet mode was requested).
func Parse(parser *kong.Kong, args []string, usageCode int) (kctx *kong.Context, code int, done bool) {
	defer func() {
		if r := recover(); r != nil {
			sig, ok := r.(ExitSignal)
			if !ok {
				panic(r)
			}
			kctx, code, done = nil, int(sig), true
		}
	}()
	kctx, err := parser.Parse(args)
	if err != nil {
		if !QuietRequested(args) {
			var pe *kong.ParseError
			if errors.As(err, &pe) && pe.Context != nil {
				_ = pe.Context.PrintUsage(true) //nolint:errcheck // usage on stderr is best effort
			}
			_, _ = fmt.Fprintf(parser.Stderr, "%s: error: %v\n", parser.Model.Name, err)
		}
		return nil, usageCode, true
	}
	return kctx, 0, false
}

// QuietRequested scans raw tokens for a quiet request, because a parse
// failure happens before any flag takes effect.
func QuietRequested(args []string) bool {
	for i, a := range args {
		if a == "--" {
			return false
		}
		name, val, hasVal := strings.Cut(a, "=")
		if name != "--stdout-format" && name != "--stdout-verbosity" {
			continue
		}
		if !hasVal && i+1 < len(args) {
			val = args[i+1]
		}
		if val == "quiet" || val == "silent" || val == "0" || val == "1" {
			return true
		}
	}
	return false
}

// FlagOverrides maps explicitly given flags to config keys: root flags
// stay flat ("stdout_format"), command flags nest under the command
// path ("list.all"). Defaults never override lower layers.
func FlagOverrides(kctx *kong.Context) map[string]any {
	out := map[string]any{}
	selected := kctx.Selected()
	for _, p := range kctx.Path {
		if p.Flag == nil || p.Flag.Name == "config" || p.Flag.Name == "env-file" || p.Flag.Name == "help" {
			continue
		}
		key := strings.ReplaceAll(p.Flag.Name, "-", "_")
		if owner := ownerPath(selected, p.Flag); owner != "" {
			key = owner + "." + key
		}
		out[key] = kctx.FlagValue(p.Flag)
	}
	return out
}

// ownerPath is the dotted path of the selected command when it declares
// f itself; flags inherited from the root resolve to "".
func ownerPath(n *kong.Node, f *kong.Flag) string {
	if n == nil {
		return ""
	}
	for _, own := range n.Flags {
		if own == f {
			var parts []string
			for x := n; x != nil && x.Parent != nil; x = x.Parent {
				parts = append([]string{x.Name}, parts...)
			}
			return strings.Join(parts, ".")
		}
	}
	return ""
}
