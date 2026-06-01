package noticecache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTripAndFreshness(t *testing.T) {
	p := Path(t.TempDir(), "tool")
	if Read(p).Fresh(time.Now(), time.Hour) {
		t.Fatal("missing cache is fresh")
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	Write(p, Entry{CheckedAt: at, Latest: "1.2.0"})
	e := Read(p)
	if e.Latest != "1.2.0" || !e.Fresh(at.Add(time.Minute), time.Hour) || e.Fresh(at.Add(2*time.Hour), time.Hour) {
		t.Errorf("entry %+v", e)
	}
	if filepath.Base(p) != FileName {
		t.Errorf("path %q", p)
	}
}

func TestOptOutVar(t *testing.T) {
	if got := OptOutVar("my-tool2"); got != "MY_TOOL2_NO_UPDATE_NOTIFIER" {
		t.Errorf("got %q", got)
	}
}

func TestPathWithoutDir(t *testing.T) {
	if got := Path("", "tool"); got != "" {
		t.Errorf("Path(\"\") = %q, want empty", got)
	}
}
