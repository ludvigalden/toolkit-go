package exportio

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestScanRequiresHeaderFirst(t *testing.T) {
	err := Scan(strings.NewReader(`{"kind":"release"}`+"\n"), "f", func(string, []byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "want header") {
		t.Errorf("err = %v", err)
	}
	if err := Scan(strings.NewReader("\n\n"), "f", nil); err == nil {
		t.Error("empty stream accepted")
	}
}

func TestReadOptional(t *testing.T) {
	fsys := fstest.MapFS{"a.json": {Data: []byte(`{"n":1}`)}}
	var v struct{ N int }
	if ok, err := ReadOptional(fsys, "a.json", "a.json", &v); !ok || err != nil || v.N != 1 {
		t.Errorf("ok=%v err=%v v=%v", ok, err, v)
	}
	if ok, err := ReadOptional(fsys, "b.json", "b.json", &v); ok || err != nil {
		t.Errorf("missing file: ok=%v err=%v", ok, err)
	}
}
