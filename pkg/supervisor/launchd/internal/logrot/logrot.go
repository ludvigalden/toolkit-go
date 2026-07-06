// Package logrot rotates a live log into a bounded set of generations.
package logrot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Rotate moves live to live.1 (shifting older generations, dropping any
// beyond keep) once it reaches threshold bytes; smaller or absent logs
// are left alone.
func Rotate(live string, threshold int64, keep int) error {
	info, err := os.Stat(live)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("supervisor: rotate log: %w", err)
	}
	if info.Size() < threshold || keep < 1 {
		return nil
	}
	if err := os.Remove(gen(live, keep)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("supervisor: rotate log: %w", err)
	}
	for g := keep; g > 1; g-- {
		if err := os.Rename(gen(live, g-1), gen(live, g)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("supervisor: rotate log: %w", err)
		}
	}
	if err := os.Rename(live, gen(live, 1)); err != nil {
		return fmt.Errorf("supervisor: rotate log: %w", err)
	}
	removeBeyond(live, keep)
	return nil
}

func gen(live string, g int) string { return live + "." + strconv.Itoa(g) }

// removeBeyond deletes generations past keep left by a lowered limit.
func removeBeyond(live string, keep int) {
	dir, base := filepath.Dir(live), filepath.Base(live)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		suffix, ok := strings.CutPrefix(e.Name(), base+".")
		if !ok {
			continue
		}
		if g, err := strconv.Atoi(suffix); err == nil && g > keep {
			_ = os.Remove(filepath.Join(dir, e.Name())) //nolint:errcheck // best-effort sweep
		}
	}
}
