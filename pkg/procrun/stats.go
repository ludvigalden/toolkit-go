package procrun

import (
	"fmt"

	"github.com/ludvigalden/toolkit-go/pkg/procrun/internal/pstat"
)

// Stats is a one-shot resource snapshot of one process.
type Stats struct {
	PID int `json:"pid"`
	// Name is the executable name; empty when the platform cannot report it.
	Name     string `json:"name,omitempty"`
	RSSBytes uint64 `json:"rss_bytes"`
	// CPUPercent is the lifetime average CPU use (100 = one core).
	CPUPercent float64 `json:"cpu_percent"`
	Threads    int32   `json:"threads"`
}

// ReadStats samples the executable name, RSS, lifetime-average CPU
// percent, and thread count for pid. Fields the platform cannot
// report stay zero.
func ReadStats(pid int) (Stats, error) {
	s, err := pstat.Read(pid)
	if err != nil {
		return Stats{PID: pid}, fmt.Errorf("procrun: stats %d: %w", pid, err)
	}
	return Stats{PID: pid, Name: s.Name, RSSBytes: s.RSSBytes, CPUPercent: s.CPUPercent, Threads: s.Threads}, nil
}

// Descendants returns the PIDs of every live descendant of pid,
// parents before children, from one snapshot of the process table.
func Descendants(pid int) ([]int, error) {
	parents, err := pstat.Parents()
	if err != nil {
		return nil, fmt.Errorf("procrun: descendants %d: %w", pid, err)
	}
	return pstat.Descend(parents, pid), nil
}

// ReadTree samples pid and every live descendant, root first;
// processes that exit mid-walk are skipped. It fails only when pid
// itself cannot be read.
func ReadTree(pid int) ([]Stats, error) {
	root, err := ReadStats(pid)
	if err != nil {
		return nil, err
	}
	kids, err := Descendants(pid)
	if err != nil {
		return []Stats{root}, fmt.Errorf("procrun: tree %d: %w", pid, err)
	}
	out := []Stats{root}
	for _, kid := range kids {
		if s, err := ReadStats(kid); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}
