// Package store holds the catalog's raw entries; internal so callers
// depend on catalog.Item, never on the storage shape.
package store

// Entry is one raw key/value pair.
type Entry struct{ Key, Value string }

// Entries returns the built-in entries in storage order.
func Entries() []Entry {
	return []Entry{
		{"widget", "A small, reusable part."},
		{"gadget", "A clever device."},
		{"bolt", "A threaded fastener."},
	}
}
