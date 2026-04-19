package argvrun

import (
	"context"
	"strings"
	"testing"
)

func TestOutput(t *testing.T) {
	ctx := context.Background()
	if got, err := Output(ctx, []string{"printf", "x\n\n"}, nil, ""); err != nil || got != "x" {
		t.Fatalf("Output = %q %v", got, err)
	}
	if _, err := Output(ctx, []string{"sh", "-c", "echo oops >&2; exit 3"}, nil, ""); err == nil || !strings.Contains(err.Error(), "oops") {
		t.Fatalf("stderr not folded: %v", err)
	}
	if _, err := Output(ctx, nil, nil, ""); err == nil {
		t.Fatal("empty argv accepted")
	}
}
