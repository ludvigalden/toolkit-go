// Package app wires the notes CLI: the Kong grammar and the cli.App
// that loads configuration and sets up output. It is internal to the
// binary.
package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/knadh/koanf/v2"

	"github.com/ludvigalden/toolkit-go/pkg/cli"
	"github.com/ludvigalden/toolkit-go/pkg/output"

	"example.com/minimal_cli/pkg/go/notes"
)

// keyDir is the tool's own config key: notes.dir in notes.yaml,
// NOTES_DIR in the environment. Empty means the per-tool data dir.
const keyDir = "notes.dir"

// Root is the Kong grammar. GlobalFlags adds the shared output,
// verbosity, and config flags every tool carries.
type Root struct {
	cli.GlobalFlags

	Add  AddCmd  `cmd:"" help:"Add a note."`
	List ListCmd `cmd:"" help:"List notes."`
}

// store resolves the note store from configuration.
func store(k *koanf.Koanf, g *cli.Global) notes.Store {
	dir := k.String(keyDir)
	if dir == "" {
		dir = filepath.Join(g.Dirs.Data, "store")
	}
	return notes.Store{Dir: dir}
}

// AddCmd appends one note.
type AddCmd struct {
	Text []string `arg:"" help:"The note text."`
}

// Run adds the note and emits it as the stdout payload.
func (c *AddCmd) Run(out *output.Output, k *koanf.Koanf, g *cli.Global) error {
	n, err := store(k, g).Add(strings.Join(c.Text, " "), time.Now())
	if err != nil {
		return err
	}
	out.Result(n)
	return nil
}

// ListCmd prints every note.
type ListCmd struct{}

// Run emits the notes as the stdout payload; the count goes to stderr.
func (c *ListCmd) Run(out *output.Output, k *koanf.Koanf, g *cli.Global) error {
	all, err := store(k, g).List()
	if err != nil {
		return err
	}
	out.Info("listed %d notes", len(all))
	if all == nil {
		all = []notes.Note{}
	}
	out.Result(all)
	return nil
}

// Main parses args, loads configuration, runs the selected command,
// and returns the process exit code.
func Main(args []string) int {
	app := &cli.App{
		Name:        "notes",
		Description: "Keep one-line notes.",
		Defaults:    map[string]any{keyDir: ""},
	}
	return app.Main(&Root{}, args)
}

