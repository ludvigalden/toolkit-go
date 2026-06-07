package gh

import (
	"context"
	"errors"
	"testing"
)

func TestDecode(t *testing.T) {
	r, err := DecodeRelease([]byte(`{"tagName":"v1.0.0","isPrerelease":true,"publishedAt":"2026-01-02T00:00:00Z"}`))
	if err != nil || r.TagName != "v1.0.0" || !r.IsPrerelease || r.PublishedAt.Year() != 2026 {
		t.Errorf("release %+v err %v", r, err)
	}
	if _, err := DecodeList([]byte(`{`)); err == nil {
		t.Error("bad list accepted")
	}
}

func TestExecMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Exec(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}
