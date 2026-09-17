package pstat

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// lifetimeCPU converts ps(1) cputime and etime into the lifetime average
// CPU percentage (100 = one core) that Sample.CPUPercent documents; ps
// %cpu alone is a decaying recent average instead.
func lifetimeCPU(cputime, etime string) (float64, error) {
	cpu, err := parsePSTime(cputime)
	if err != nil {
		return 0, fmt.Errorf("cputime %q: %w", cputime, err)
	}
	elapsed, err := parsePSTime(etime)
	if err != nil {
		return 0, fmt.Errorf("etime %q: %w", etime, err)
	}
	if elapsed <= 0 {
		return 0, nil
	}
	return 100 * cpu.Seconds() / elapsed.Seconds(), nil
}

// parsePSTime parses a ps(1) duration: an optional "D-" day prefix,
// colon-separated units one to three wide (SS, MM:SS, HH:MM:SS), and an
// optional centisecond fraction on the last field.
func parsePSTime(s string) (time.Duration, error) {
	if s == "" {
		return 0, errors.New("empty duration")
	}
	days := time.Duration(0)
	if i := strings.IndexByte(s, '-'); i >= 0 {
		d, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, fmt.Errorf("day prefix: %w", err)
		}
		days = time.Duration(d) * 24 * time.Hour
		s = s[i+1:]
	}
	frac := time.Duration(0)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		f, err := strconv.ParseFloat("0."+s[i+1:], 64)
		if err != nil {
			return 0, fmt.Errorf("fraction: %w", err)
		}
		frac = time.Duration(f * float64(time.Second))
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("too many fields in %q", s)
	}
	total := days + frac
	units := []time.Duration{time.Second, time.Minute, time.Hour}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("field %q: %w", p, err)
		}
		total += time.Duration(n) * units[len(parts)-1-i]
	}
	return total, nil
}
