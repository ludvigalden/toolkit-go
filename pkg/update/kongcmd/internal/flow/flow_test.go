package flow

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/update"
)

type stub struct {
	rel update.Release
	err error
}

func (s stub) Latest(context.Context) (update.Release, error) { return s.rel, s.err }

func TestVersionUnavailableCheckerIsSilent(t *testing.T) {
	var out, errb bytes.Buffer
	o := IO{Ctx: context.Background(), Tool: "t", Stdout: &out, Stderr: &errb}
	if err := Version(o, "1.0.0", nil, true, stub{err: update.ErrUnavailable}); err != nil || out.String() != "t 1.0.0\n" || errb.Len() != 0 {
		t.Errorf("err=%v out=%q stderr=%q", err, out.String(), errb.String())
	}
}

func TestUpdateCheckJSON(t *testing.T) {
	var out bytes.Buffer
	o := IO{Ctx: context.Background(), Tool: "t", Stdout: &out, Stderr: &out, JSON: true}
	if err := Update(o, "1.0.0", true, stub{rel: update.Release{Version: "1.1.0"}}, nil, nil); err != nil || !strings.Contains(out.String(), `"available": true`) {
		t.Errorf("err=%v out=%s", err, out.String())
	}
	sentinel := errors.New("unavailable")
	if err := Update(o, "1.0.0", false, nil, nil, sentinel); !errors.Is(err, sentinel) {
		t.Errorf("no updater: %v", err)
	}
}
