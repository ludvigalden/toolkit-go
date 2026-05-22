package merge

import "testing"

func TestFilter(t *testing.T) {
	rels := []Embedded{{"1.3.0-rc.1", false, 0}, {"1.2.0", true, 1}, {"1.1.0", true, 2}}
	kept, newest := Filter(rels, Options{Since: "1.1.0"})
	if newest != "1.3.0-rc.1" || len(kept) != 1 || kept[0].Ref != 1 {
		t.Errorf("kept=%v newest=%q", kept, newest)
	}
	if kept, _ = Filter(rels, Options{Prereleases: true}); len(kept) != 3 {
		t.Errorf("prereleases: %v", kept)
	}
}

func TestFloor(t *testing.T) {
	for _, c := range [][3]string{{"", "", "0.0.0"}, {"1.2.0", "", "1.2.0"}, {"1.2.0", "1.5.0", "1.5.0"}, {"1.2.0", "1.0.0", "1.2.0"}, {"", "0.9.0", "0.9.0"}} {
		if got := Floor(c[0], c[1]); got != c[2] {
			t.Errorf("Floor(%q,%q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
	if Cap(5, 3) != 3 || Cap(2, 3) != 2 || Cap(5, 0) != 5 {
		t.Error("Cap")
	}
}
