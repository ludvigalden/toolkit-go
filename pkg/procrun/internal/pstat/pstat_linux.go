package pstat

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"strconv"
	"strings"
)

// clockTicks is USER_HZ, fixed at 100 on every Linux ABI Go supports.
const clockTicks = 100

// Read parses /proc/<pid>/stat and /proc/<pid>/status.
func Read(pid int) (Sample, error) {
	base := "/proc/" + strconv.Itoa(pid)
	stat, err := os.ReadFile(base + "/stat")
	if err != nil {
		return Sample{}, err //nolint:wrapcheck // wrapped by procrun.ReadStats
	}
	// The parenthesised command name may hold spaces and parens, so the
	// field split starts after its final ')'.
	open := bytes.IndexByte(stat, '(')
	end := bytes.LastIndexByte(stat, ')')
	if open < 0 || end < 0 || open > end {
		return Sample{}, errors.New("malformed stat")
	}
	f := strings.Fields(string(stat[end+1:]))
	if len(f) < 22 {
		return Sample{}, errors.New("short stat")
	}
	var s Sample
	s.Name = string(stat[open+1 : end])
	if n, err := strconv.ParseInt(f[17], 10, 32); err == nil { // num_threads
		s.Threads = int32(n)
	}
	utime, _ := strconv.ParseFloat(f[11], 64) //nolint:errcheck // zero on parse failure
	stime, _ := strconv.ParseFloat(f[12], 64) //nolint:errcheck // zero on parse failure
	start, _ := strconv.ParseFloat(f[19], 64) //nolint:errcheck // zero on parse failure
	if up := uptime(); up > 0 && start > 0 {
		if alive := up - start/clockTicks; alive > 0 {
			s.CPUPercent = 100 * (utime + stime) / clockTicks / alive
		}
	}
	s.RSSBytes = statusKB(base+"/status", "VmRSS:") * 1024
	return s, nil
}

// Parents maps each live pid to its parent pid. Entries that vanish
// mid-scan are skipped; the scan is a snapshot, not a transaction.
func Parents() (map[int]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by procrun.Descendants
	}
	out := make(map[int]int)
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		end := bytes.LastIndexByte(b, ')')
		if end < 0 {
			continue
		}
		f := strings.Fields(string(b[end+1:]))
		if len(f) < 2 {
			continue
		}
		if ppid, err := strconv.Atoi(f[1]); err == nil {
			pid, _ := strconv.Atoi(e.Name())
			out[pid] = ppid
		}
	}
	return out, nil
}

// uptime returns system uptime in seconds, or 0.
func uptime() float64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64) //nolint:errcheck // zero on parse failure
	return v
}

// statusKB returns the kB value of key in a /proc status file, or 0.
func statusKB(path, key string) uint64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), key); ok {
			if fs := strings.Fields(rest); len(fs) > 0 {
				v, _ := strconv.ParseUint(fs[0], 10, 64) //nolint:errcheck // zero on parse failure
				return v
			}
		}
	}
	return 0
}
