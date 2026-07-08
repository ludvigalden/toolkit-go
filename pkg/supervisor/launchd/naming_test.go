package launchd

import (
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

func TestNaming(t *testing.T) {
	spec := supervisor.Spec{Namespace: "com.example", UnitName: "demo:2", Port: 443}
	if l, err := Label(spec); err != nil || l != "com.example.demo.443" {
		t.Fatalf("Label = %q, %v", l, err)
	}
	if f, err := PlistFileName(spec); err != nil || f != "com.example.demo.443.plist" {
		t.Fatalf("PlistFileName = %q, %v", f, err)
	}
	if _, err := Label(supervisor.Spec{UnitName: "demo"}); err == nil {
		t.Fatal("empty namespace accepted")
	}
}

func TestDefaultLogDir(t *testing.T) {
	getenv := func(k string) string {
		return map[string]string{"HOME": "/Users/u", "XDG_STATE_HOME": "/state"}[k]
	}
	if d, err := DefaultLogDir("demo", getenv); err != nil || d != "/state/demo/logs" {
		t.Fatalf("DefaultLogDir = %q, %v", d, err)
	}
	if _, err := DefaultLogDir("../x", getenv); err == nil {
		t.Fatal("path-like app accepted")
	}
}
