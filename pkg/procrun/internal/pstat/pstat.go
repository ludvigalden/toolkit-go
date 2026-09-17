// Package pstat samples one process's resource use: /proc on Linux,
// ps(1) on macOS.
package pstat

import "sort"

// Sample is one resource snapshot. Fields the platform cannot report
// stay zero; CPUPercent is the lifetime average (100 = one core).
type Sample struct {
	Name       string
	RSSBytes   uint64
	CPUPercent float64
	Threads    int32
}

// Descend walks the parent map outward from root, breadth first.
// Sorting the children up front keeps the result deterministic
// despite the map.
func Descend(parents map[int]int, root int) []int {
	children := make(map[int][]int)
	for pid, ppid := range parents {
		children[ppid] = append(children[ppid], pid)
	}
	for _, cs := range children {
		sort.Ints(cs)
	}
	var out []int
	seen := map[int]bool{root: true}
	level := []int{root}
	for len(level) > 0 {
		var next []int
		for _, p := range level {
			for _, c := range children[p] {
				if seen[c] {
					continue
				}
				seen[c] = true
				out = append(out, c)
				next = append(next, c)
			}
		}
		level = next
	}
	return out
}
