package conns

import (
	"testing"
	"time"
)

func TestTrackerAdmitDrain(t *testing.T) {
	tr := New(time.Now())
	if !tr.Admit() || tr.Idle() {
		t.Fatal("admit did not count")
	}
	tr.StartDrain()
	if tr.Admit() {
		t.Fatal("admitted while draining")
	}
	canceled := false
	go func() { time.Sleep(20 * time.Millisecond); tr.Done() }()
	tr.Wait(time.Second, func() { canceled = true })
	if n, _, draining := tr.Snapshot(); n != 0 || !draining || canceled {
		t.Fatalf("snapshot n=%d draining=%v canceled=%v", n, draining, canceled)
	}
}

func TestLeastRecent(t *testing.T) {
	now := time.Now()
	idle := []Idler{{"b", now}, {"a", now.Add(-time.Minute)}, {"c", now.Add(-time.Minute)}}
	if got := LeastRecent(idle, 2); got != 1 {
		t.Fatalf("LeastRecent = %d", got)
	}
	if LeastRecent(idle, 4) != -1 || LeastRecent(idle, 0) != -1 {
		t.Fatal("limit not honored")
	}
}
