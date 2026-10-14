// Package catalog is a specialized public API of toolname: the items
// the tool lists and shows. Storage lives in catalog/internal.
package catalog

import (
	"slices"
	"strings"

	"example.com/toolname/pkg/go/toolname/catalog/internal/store"
)

// Item is one catalog entry.
type Item struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Catalog reads items. One method: callers that only list need nothing
// else.
type Catalog interface {
	Items() []Item
}

// Builtin returns the catalog compiled into the binary.
func Builtin() Catalog { return builtin{} }

type builtin struct{}

func (builtin) Items() []Item {
	raw := store.Entries()
	out := make([]Item, 0, len(raw))
	for _, e := range raw {
		out = append(out, Item{Name: e.Key, Description: e.Value})
	}
	slices.SortFunc(out, func(a, b Item) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Find returns the item called name.
func Find(c Catalog, name string) (Item, bool) {
	for _, it := range c.Items() {
		if it.Name == name {
			return it, true
		}
	}
	return Item{}, false
}
