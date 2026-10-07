// Package notes stores one-line notes in a JSON Lines file. It is the
// tool's public API: the CLI is a thin layer over it.
package notes

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Note is one stored note.
type Note struct {
	ID      int       `json:"id"`
	Text    string    `json:"text"`
	Created time.Time `json:"created"`
}

// Store reads and appends notes under Dir.
type Store struct {
	Dir string
}

func (s Store) path() string { return filepath.Join(s.Dir, "notes.jsonl") }

// List returns every stored note, oldest first. A missing file is an
// empty store, not an error.
func (s Store) List() ([]Note, error) {
	f, err := os.Open(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Note
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var n Note
		if err := json.Unmarshal(sc.Bytes(), &n); err != nil {
			return nil, fmt.Errorf("%s: %w", s.path(), err)
		}
		out = append(out, n)
	}
	return out, sc.Err()
}

// Add appends a note and returns it with its assigned ID.
func (s Store) Add(text string, now time.Time) (Note, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Note{}, errors.New("a note needs text")
	}
	existing, err := s.List()
	if err != nil {
		return Note{}, err
	}
	n := Note{ID: len(existing) + 1, Text: text, Created: now.UTC()}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return Note{}, err
	}
	f, err := os.OpenFile(s.path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return Note{}, err
	}
	line, _ := json.Marshal(n)
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return Note{}, err
	}
	return n, f.Close()
}
