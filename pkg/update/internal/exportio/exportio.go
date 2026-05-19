// Package exportio reads the files of a relmap export: it scans
// changelog.ndjson record by record and decodes the optional JSON
// side files, leaving record types to the caller.
package exportio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
)

// Scan calls fn for each non-blank line of r with its record kind and
// raw JSON. The first record must be a header; an empty stream is an
// error. Errors name the file and 1-based record number.
func Scan(r io.Reader, file string, fn func(kind string, raw []byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for sc.Scan() {
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		var kind struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(b, &kind); err != nil {
			return fmt.Errorf("relmapx: %s line %d: %w", file, line+1, err)
		}
		if line == 0 && kind.Kind != "header" {
			return fmt.Errorf("relmapx: %s: first record is %q, want header", file, kind.Kind)
		}
		if err := fn(kind.Kind, b); err != nil {
			return fmt.Errorf("relmapx: %s line %d: %w", file, line+1, err)
		}
		line++
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("relmapx: read %s: %w", file, err)
	}
	if line == 0 {
		return fmt.Errorf("relmapx: %s is empty", file)
	}
	return nil
}

// ReadOptional decodes the JSON file name in fsys into v. found is
// false, with no error, when the file does not exist.
func ReadOptional(fsys fs.FS, name, file string, v any) (found bool, err error) {
	b, err := fs.ReadFile(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("relmapx: read %s: %w", file, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("relmapx: decode %s: %w", file, err)
	}
	return true, nil
}
