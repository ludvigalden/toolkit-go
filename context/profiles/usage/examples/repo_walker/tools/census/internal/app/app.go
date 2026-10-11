// Package app wires the census CLI.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/alecthomas/kong"

	"github.com/ludvigalden/toolkit-go/pkg/exitcode"

	"example.com/repo_walker/pkg/go/census"
)

// Root is the grammar.
type Root struct {
	Dir     string   `arg:"" optional:"" help:"Repository root." default:"." type:"existingdir"`
	Skip     []string `help:"Extra directory names to skip at any depth."`
	NoIgnore bool     `help:"Count gitignored files too."`
	JSON    bool     `help:"Write one JSON document."`
}

// Main runs the tool and returns its exit code.
func Main(args []string) int {
	var root Root
	parser, err := kong.New(&root, kong.Name("census"), kong.Description("Count a repository's files by extension."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "census:", err)
		return int(exitcode.Software)
	}
	if _, err := parser.Parse(args); err != nil {
		parser.FatalIfErrorf(err)
	}
	rep, err := census.Take(context.Background(), os.DirFS(root.Dir), census.Options{
		SkipDirs: root.Skip, NoIgnore: root.NoIgnore,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "census:", err)
		return int(exitcode.Failure)
	}
	if root.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return int(exitcode.Failure)
		}
		return int(exitcode.OK)
	}
	fmt.Printf("%d files, %d bytes\n\n", rep.Files, rep.Bytes)
	for _, k := range census.Sorted(rep.Extensions) {
		fmt.Printf("  %-12s %d\n", k, rep.Extensions[k])
	}
	return int(exitcode.OK)
}
