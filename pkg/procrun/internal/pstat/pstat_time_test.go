package pstat

import (
	"math"
	"testing"
	"time"
)

func TestParsePSTime(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"42", 42 * time.Second},
		{"0:59", 59 * time.Second},
		{"12:34", 12*time.Minute + 34*time.Second},
		{"1:02:03", time.Hour + 2*time.Minute + 3*time.Second},
		{"0:01.23", 1230 * time.Millisecond},
		{"1:02:03.45", time.Hour + 2*time.Minute + 3*time.Second + 450*time.Millisecond},
		{"3-04:05:06", 3*24*time.Hour + 4*time.Hour + 5*time.Minute + 6*time.Second},
		{"2-0:00.5", 48*time.Hour + 500*time.Millisecond},
	}
	for _, c := range cases {
		got, err := parsePSTime(c.in)
		if err != nil || got != c.want {
			t.Errorf("parsePSTime(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "x", "1:2:3:4", "1:x", "1-.5", "-1:00"} {
		if _, err := parsePSTime(bad); err == nil {
			t.Errorf("parsePSTime(%q) accepted", bad)
		}
	}
}

func TestLifetimeCPU(t *testing.T) {
	cases := []struct {
		cputime, etime string
		want           float64
	}{
		{"0:30", "0:01:00", 50},
		{"0:01:30", "0:00:45", 200},
		{"0:00.50", "0:00:10", 5},
		{"0:00", "0:00:00", 0}, // just started: no elapsed time to average over
	}
	for _, c := range cases {
		got, err := lifetimeCPU(c.cputime, c.etime)
		if err != nil || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("lifetimeCPU(%q, %q) = %v, %v; want %v", c.cputime, c.etime, got, err, c.want)
		}
	}
	for _, bad := range [][2]string{{"junk", "0:01:00"}, {"0:30", "junk"}} {
		if _, err := lifetimeCPU(bad[0], bad[1]); err == nil {
			t.Errorf("lifetimeCPU(%q, %q) accepted", bad[0], bad[1])
		}
	}
}
