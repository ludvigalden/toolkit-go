package pstat

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Read asks ps(1), which ships with every macOS install; the
// kernel interfaces need cgo or private structs. CPUPercent comes from
// cputime/etime because ps %cpu is a decaying recent average, not the
// lifetime average Sample documents.
func Read(pid int) (Sample, error) {
	out, err := exec.Command("ps", "-o", "rss=,cputime=,etime=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return Sample{}, err //nolint:wrapcheck // wrapped by procrun.ReadStats
	}
	f := strings.Fields(string(out))
	if len(f) < 3 {
		return Sample{}, errors.New("no such process")
	}
	var s Sample
	if kb, err := strconv.ParseUint(f[0], 10, 64); err == nil {
		s.RSSBytes = kb * 1024
	}
	// A shape mismatch across ps releases must not fail Read; the field
	// simply stays zero.
	if cpu, err := lifetimeCPU(f[1], f[2]); err == nil {
		s.CPUPercent = cpu
	}
	// comm is last and may hold spaces, so it is rejoined here.
	if len(f) >= 4 {
		s.Name = filepath.Base(strings.Join(f[3:], " "))
	}
	if th, err := exec.Command("ps", "-M", "-p", strconv.Itoa(pid)).Output(); err == nil {
		if n := strings.Count(strings.TrimSpace(string(th)), "\n"); n > 0 {
			s.Threads = int32(n) //nolint:gosec // thread counts fit int32
		}
	}
	return s, nil
}

// Parents maps every pid to its parent pid via one ps(1) listing.
func Parents() (map[int]int, error) {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by procrun.Descendants
	}
	m := make(map[int]int)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			m[pid] = ppid
		}
	}
	return m, nil
}
