// Package lexer scans .env bytes into assignments: KEY=VALUE lines,
// comments, an optional export prefix, and single or double quoting.
package lexer

import (
	"errors"
	"fmt"
	"strings"
)

// Assignment is one parsed KEY=VALUE; Line is the 1-based key line.
type Assignment struct {
	Key   string
	Value string
	Line  int
}

// Error reports the first malformed line.
type Error struct {
	Line int
	Msg  string
}

// Error renders "line N: msg".
func (e *Error) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// Parse returns data's assignments in order, duplicates included; the
// first malformed line fails the whole parse with *Error.
func Parse(data []byte) ([]Assignment, error) {
	var out []Assignment
	line := 1
	pos := 0 // offset of the next unparsed line start
	for pos < len(data) {
		end := pos
		for end < len(data) && data[end] != '\n' {
			end++
		}
		raw := string(data[pos:end])
		head := strings.TrimLeft(raw, " \t\r")
		if strings.TrimRight(head, " \t\r") == "" || head[0] == '#' {
			pos, line = end+1, line+1
			continue
		}
		fail := func(format string, args ...any) error {
			return &Error{Line: line, Msg: fmt.Sprintf(format, args...)}
		}
		keyLen := strings.IndexAny(head, "='\"")
		if keyLen < 0 || head[keyLen] != '=' {
			return nil, fail("expected KEY=VALUE, got %q", strings.TrimSpace(raw))
		}
		key := head[:keyLen]
		// "export KEY=..." strips one export word; "exportKEY" stays a key.
		if word, rest, ok := strings.Cut(key, " "); ok && word == "export" {
			key = strings.TrimLeft(rest, " \t")
		}
		key = strings.TrimRight(key, " \t")
		if !validKey(key) {
			return nil, fail("%q is not a valid key ([A-Za-z_][A-Za-z0-9_]*)", key)
		}
		valAt := pos + (len(raw) - len(head)) + keyLen + 1 // byte after '='
		for valAt < len(data) && (data[valAt] == ' ' || data[valAt] == '\t') {
			valAt++
		}
		var (
			value    string
			consumed int
			err      error
		)
		switch {
		case valAt < len(data) && data[valAt] == '"':
			value, consumed, err = doubleQuoted(data[valAt:])
		case valAt < len(data) && data[valAt] == '\'':
			value, consumed, err = singleQuoted(data[valAt:])
		default:
			if end < valAt {
				end = valAt
			}
			value = strings.TrimRight(string(data[valAt:end]), " \t\r")
			// A value whose first non-space character is # is a
			// comment, so the entry binds an empty value; mid-value
			// " #" is still an inline comment.
			if strings.HasPrefix(value, "#") {
				value = ""
			}
			if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimRight(value[:i], " \t")
			}
			consumed = end - valAt
		}
		if err != nil {
			return nil, fail("%s", err.Error())
		}
		tailEnd := valAt + consumed
		for tailEnd < len(data) && data[tailEnd] != '\n' {
			tailEnd++
		}
		if tail := strings.TrimSpace(string(data[valAt+consumed : tailEnd])); tail != "" && tail[0] != '#' {
			return nil, fail("unexpected %q after the value", tail)
		}
		out = append(out, Assignment{Key: key, Value: value, Line: line})
		line += strings.Count(string(data[pos:tailEnd]), "\n") + 1
		pos = tailEnd + 1
	}
	return out, nil
}

func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// singleQuoted returns the literal body up to the closing quote and the
// bytes consumed including both quotes; newlines are kept.
func singleQuoted(data []byte) (string, int, error) {
	for i := 1; i < len(data); i++ {
		if data[i] == '\'' {
			return string(data[1:i]), i + 1, nil
		}
	}
	return "", 0, errors.New("unterminated single-quoted value")
}

// doubleQuoted decodes \n \r \t and backslash-escaped characters; a
// backslash before a newline joins lines, a bare newline is kept.
func doubleQuoted(data []byte) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(data); i++ {
		switch c := data[i]; c {
		case '"':
			return b.String(), i + 1, nil
		case '\\':
			if i+1 >= len(data) {
				return "", 0, errors.New("unterminated double-quoted value")
			}
			i++
			switch n := data[i]; n {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\n':
			default:
				b.WriteByte(n)
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, errors.New("unterminated double-quoted value")
}
