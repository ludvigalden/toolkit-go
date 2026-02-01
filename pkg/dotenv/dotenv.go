// Package dotenv parses .env files deterministically: KEY=VALUE lines,
// comments, an optional export prefix and quoting. Nothing is expanded
// or executed, so loading never depends on the environment.
package dotenv

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/dotenv/internal/lexer"
)

// Entry is one assignment. Line is the 1-based line of the key.
type Entry struct {
	Key   string
	Value string
	Line  int
}

// ParseError reports a malformed line.
type ParseError struct {
	File string // empty when parsing a reader
	Line int
	Msg  string
}

// Error renders "file:line: msg", or "line N: msg" without a file.
func (e *ParseError) Error() string {
	if e.File != "" {
		return fmt.Sprintf("dotenv: %s:%d: %s", e.File, e.Line, e.Msg)
	}
	return fmt.Sprintf("dotenv: line %d: %s", e.Line, e.Msg)
}

// ParseFile parses the file at path; parse errors carry the path.
func ParseFile(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dotenv: %w", err)
	}
	return parse(data, path)
}

// Parse reads r to the end and returns its entries in file order,
// duplicates included. The first malformed line fails the whole parse.
func Parse(r io.Reader) ([]Entry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("dotenv: read: %w", err)
	}
	return parse(data, "")
}

// ToMap folds entries into a map; a later duplicate key wins.
func ToMap(entries []Entry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Key] = e.Value
	}
	return m
}

// parse runs the lexer and maps its result to the public types.
func parse(data []byte, file string) ([]Entry, error) {
	as, err := lexer.Parse(data)
	var le *lexer.Error
	if errors.As(err, &le) {
		return nil, &ParseError{File: file, Line: le.Line, Msg: le.Msg}
	}
	var out []Entry
	for _, a := range as {
		out = append(out, Entry{Key: a.Key, Value: a.Value, Line: a.Line})
	}
	return out, nil
}
