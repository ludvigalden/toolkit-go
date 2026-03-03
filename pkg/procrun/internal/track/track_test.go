package track

import (
	"os/exec"
	"testing"
	"time"
)

func TestCleanupKillsTracked(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := New()
	s.Add(cmd.Process, LongRunning)
	if s.Len() != 1 {
		t.Fatalf("Len = %d", s.Len())
	}
	s.Cleanup(50 * time.Millisecond)
	if err := cmd.Wait(); err == nil {
		t.Fatal("tracked process exited cleanly")
	}
	if s.Len() != 0 {
		t.Fatal("Cleanup kept entries")
	}
	var nilSet *Set
	nilSet.Add(cmd.Process, OneShot)
}
