package wiring

import (
	"log/slog"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/output"
)

func TestEnvPrefix(t *testing.T) {
	for _, c := range [][3]string{{"my-tool", "", "MY_TOOL_"}, {"x", "CUSTOM_", "CUSTOM_"}, {"x", "CUSTOM", "CUSTOM_"}} {
		if got := EnvPrefix(c[0], c[1]); got != c[2] {
			t.Errorf("EnvPrefix(%q,%q) = %q", c[0], c[1], got)
		}
	}
}

func TestLevelForAndSessionID(t *testing.T) {
	if LevelFor(output.VerbosityWarn) != slog.LevelWarn || LevelFor(output.VerbosityDebug) != slog.LevelDebug {
		t.Error("LevelFor")
	}
	if id := SessionID(); len(id) != 8 {
		t.Errorf("SessionID = %q", id)
	}
}
