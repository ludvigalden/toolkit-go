// Package kongx holds the Kong plumbing behind cli: parsing without
// letting Kong exit the process, honoring a silent request before flags
// apply, and mapping explicitly given flags to config keys.
package kongx

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/ludvigalden/toolkit-go/pkg/output"
)

// ExitSignal carries Kong's exit request (after --help) out of Parse
// without terminating the process; install it with kong.Exit.
type ExitSignal int

// Exit is the kong.Exit hook that raises ExitSignal.
func Exit(code int) { panic(ExitSignal(code)) }

// Parse parses args. done reports that the caller should exit with
// code: after --help, or with usageCode for bad arguments (printed to
// the parser's stderr unless silent).
func Parse(parser *kong.Kong, args []string, usageCode int, silent bool) (kctx *kong.Context, code int, done bool) {
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
		if !silent {
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

// SilentRequested reports whether verbosity silent was asked for: by
// the last --stdout-verbosity token, else by env (the variable's value).
// It scans raw tokens because a parse failure happens before any flag
// takes effect.
func SilentRequested(args []string, env string) bool {
	val, found := env, false
	for i, a := range args {
		if a == "--" {
			break
		}
		name, v, hasVal := strings.Cut(a, "=")
		if name != "--stdout-verbosity" {
			continue
		}
		if !hasVal {
			v = ""
			if i+1 < len(args) {
				v = args[i+1]
			}
		}
		val, found = v, true
	}
	if !found && val == "" {
		return false
	}
	verb, err := output.ParseVerbosity(val)
	return err == nil && verb == output.VerbositySilent
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
		} else {
			key = rootKey(key)
		}
		out[key] = kctx.FlagValue(p.Flag)
	}
	return out
}

// rootKey maps the prefixed root flags to their plain config keys.
func rootKey(key string) string {
	switch key {
	case "session_id":
		return "session"
	case "config_profile":
		return "profile"
	}
	return key
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
