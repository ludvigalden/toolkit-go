package secret

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/secret/internal/argvrun"
)

// Source fetches one secret's current value.
type Source interface {
	Fetch(ctx context.Context) (string, error)
}

// SourceFunc adapts a function to Source.
type SourceFunc func(ctx context.Context) (string, error)

// Fetch calls f.
func (f SourceFunc) Fetch(ctx context.Context) (string, error) { return f(ctx) }

// Env reads variable name through getenv (nil means os.Getenv). An
// empty value is an error, so an unset variable never resolves to "".
func Env(getenv func(string) string, name string) Source {
	if getenv == nil {
		getenv = os.Getenv
	}
	return SourceFunc(func(context.Context) (string, error) {
		if v := getenv(name); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("environment variable %s is empty", name)
	})
}

// Literal returns s; useful in tests and for defaults.
func Literal(s string) Source {
	return SourceFunc(func(context.Context) (string, error) { return s, nil })
}

// File reads path, trimming trailing whitespace.
func File(path string) Source {
	return SourceFunc(func(context.Context) (string, error) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), " \t\r\n"), nil
	})
}

// CommandSource runs Argv directly, without a shell, and uses its
// stdout minus trailing whitespace. Env nil inherits the environment.
type CommandSource struct {
	Argv []string
	Env  []string
	Dir  string
}

// Command returns a CommandSource for argv.
func Command(argv ...string) *CommandSource {
	return &CommandSource{Argv: argv}
}

// Fetch runs the command. On failure its stderr becomes part of the
// error and its stdout is discarded.
func (c *CommandSource) Fetch(ctx context.Context) (string, error) {
	return argvrun.Output(ctx, c.Argv, c.Env, c.Dir) //nolint:wrapcheck // the error names the command
}

// Vault is the plug-in point for an external secret store.
type Vault interface {
	Fetch(ctx context.Context, key string) (string, error)
}

// FromVault fetches key from v.
func FromVault(v Vault, key string) Source {
	return SourceFunc(func(ctx context.Context) (string, error) {
		return v.Fetch(ctx, key)
	})
}
