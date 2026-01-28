package lexer

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	got, err := Parse([]byte("# c\nexport A=1 # x\nB=\"x\\ny\"\nC='l\nm'\n"))
	if err != nil || len(got) != 3 {
		t.Fatalf("Parse = %+v %v", got, err)
	}
	if got[1].Value != "x\ny" || got[2].Value != "l\nm" || got[2].Line != 4 {
		t.Fatalf("values = %+v", got)
	}
	var le *Error
	if _, err := Parse([]byte("A=1\n1BAD=x\n")); !errors.As(err, &le) || le.Line != 2 {
		t.Fatalf("bad key = %v", err)
	}
}

func TestLeadingHashValue(t *testing.T) {
	got, err := Parse([]byte("A= # comment\nB=#comment\nC=' #kept'\nD=v # tail\n"))
	if err != nil {
		t.Fatalf("Parse = %v", err)
	}
	want := []string{"", "", " #kept", "v"}
	for i, w := range want {
		if got[i].Value != w {
			t.Errorf("%s = %q, want %q", got[i].Key, got[i].Value, w)
		}
	}
}
