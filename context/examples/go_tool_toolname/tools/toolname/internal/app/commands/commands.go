// Package commands holds toolname's own subcommands. Each Run receives
// what the root binds (the catalog and the output), so commands never
// reach for globals.
package commands

import (
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/output"

	"example.com/toolname/pkg/go/common/textfmt"
	"example.com/toolname/pkg/go/toolname/catalog"
)

// ListCmd lists every item.
type ListCmd struct{}

// Run emits the items as the stdout payload in the chosen mode.
func (ListCmd) Run(c catalog.Catalog, out *output.Output) error {
	out.Result(c.Items())
	return nil
}

// ShowCmd shows one item.
type ShowCmd struct {
	Name string `arg:"" help:"Item to show."`
}

// Run emits the item, or a not-found error (exit 66) when unknown.
func (s ShowCmd) Run(c catalog.Catalog, out *output.Output) error {
	it, ok := catalog.Find(c, s.Name)
	if !ok {
		return errkind.Newf(errkind.NotFound, "no item named %q", s.Name)
	}
	out.Result(card(it))
	return nil
}

// card renders an item as a wrapped paragraph in text modes and as the
// plain item in structured ones.
type card catalog.Item

func (c card) FormatText(output.Args) string {
	return c.Name + "\n  " + strings.Join(textfmt.Wrap(c.Description, 60), "\n  ")
}

func (c card) FormatData(output.Args) any { return catalog.Item(c) }
