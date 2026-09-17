package pstat

import (
	"os"
	"slices"
	"testing"
)

func TestRead(t *testing.T) {
	s, err := Read(os.Getpid())
	if err != nil || s.RSSBytes == 0 || s.Name == "" {
		t.Fatalf("self sample = %+v %v", s, err)
	}
	if _, err := Read(1 << 30); err == nil {
		t.Fatal("sample for an impossible pid succeeded")
	}
}

func TestDescend(t *testing.T) {
	parents := map[int]int{
		100: 1,
		10:  100, 20: 100, 30: 100,
		15: 10,
		40: 30,
		5:  1,  // unrelated subtree
		7:  15, // third level
	}
	got := Descend(parents, 100)
	want := []int{10, 20, 30, 15, 40, 7}
	if !slices.Equal(got, want) {
		t.Fatalf("Descend = %v, want %v", got, want)
	}
	if got := Descend(parents, 999); len(got) != 0 {
		t.Fatalf("absent root = %v", got)
	}
}
