package dotenv

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Entry
	}{
		{"empty", "", nil},
		{"blank and comments", "\n  \n# c\n\t# d\r\n", nil},
		{"simple", "A=1\nB=two", []Entry{{"A", "1", 1}, {"B", "two", 2}}},
		{"crlf", "A=1\r\nB=2\r\n", []Entry{{"A", "1", 1}, {"B", "2", 2}}},
		{"empty value", "A=\nB=", []Entry{{"A", "", 1}, {"B", "", 2}}},
		{"spaces around", "  A = x y  \n", []Entry{{"A", "x y", 1}}},
		{"export", "export A=1\nexport  B=2", []Entry{{"A", "1", 1}, {"B", "2", 2}}},
		{"export as key", "export=1", []Entry{{"export", "1", 1}}},
		{"no expansion", "A=$HOME ${X} $(id)", []Entry{{"A", "$HOME ${X} $(id)", 1}}},
		{"inline comment", "A=x # note\nB=a#b", []Entry{{"A", "x", 1}, {"B", "a#b", 2}}},
		{"single quoted", `A='a "b" \n $X'`, []Entry{{"A", `a "b" \n $X`, 1}}},
		{"single multiline", "A='x\ny'\nB=1", []Entry{{"A", "x\ny", 1}, {"B", "1", 3}}},
		{"double escapes", `A="a\nb\tc\r\"q\"\\ \$"`, []Entry{{"A", "a\nb\tc\r\"q\"\\ $", 1}}},
		{"double multiline", "A=\"x\ny\"\nB=1", []Entry{{"A", "x\ny", 1}, {"B", "1", 3}}},
		{"continuation", "A=\"x\\\ny\"\nB=1", []Entry{{"A", "xy", 1}, {"B", "1", 3}}},
		{"quoted with comment", `A="x" # c`, []Entry{{"A", "x", 1}}},
		{"duplicates kept", "A=1\nA=2", []Entry{{"A", "1", 1}, {"A", "2", 2}}},
		{"equals in value", "A=b=c", []Entry{{"A", "b=c", 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		line int
		msg  string
	}{
		{"no equals", "A=1\nJUNK", 2, "expected KEY=VALUE"},
		{"bad key", "1A=x", 1, "not a valid key"},
		{"bad key char", "A-B=x", 1, "not a valid key"},
		{"empty key", "=x", 1, "not a valid key"},
		{"quote in key", "A'=x", 1, "expected KEY=VALUE"},
		{"unterminated single", "A='x", 1, "unterminated single"},
		{"unterminated double", "\nA=\"x", 2, "unterminated double"},
		{"dangling escape", `A="x\`, 1, "unterminated double"},
		{"trailing junk", `A="x" y`, 1, "unexpected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.in))
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v, want *ParseError", err)
			}
			if pe.Line != tt.line || !strings.Contains(pe.Msg, tt.msg) {
				t.Errorf("got line %d %q, want line %d containing %q", pe.Line, pe.Msg, tt.line, tt.msg)
			}
			if !strings.HasPrefix(err.Error(), "dotenv: line ") {
				t.Errorf("Error() = %q", err.Error())
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.env")
	bad := filepath.Join(dir, "bad.env")
	if err := os.WriteFile(good, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("A=1\nnope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseFile(good)
	if err != nil || !reflect.DeepEqual(got, []Entry{{"A", "1", 1}}) {
		t.Errorf("ParseFile(good) = %v, %v", got, err)
	}
	_, err = ParseFile(bad)
	if err == nil || !strings.HasPrefix(err.Error(), "dotenv: "+bad+":2: ") {
		t.Errorf("ParseFile(bad) err = %v", err)
	}
	if _, err := ParseFile(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ParseFile(missing) err = %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParseReadError(t *testing.T) {
	if _, err := Parse(errReader{}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
}

func TestToMap(t *testing.T) {
	got := ToMap([]Entry{{"A", "1", 1}, {"B", "2", 2}, {"A", "3", 3}})
	want := map[string]string{"A": "3", "B": "2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToMap = %v, want %v", got, want)
	}
	if m := ToMap(nil); m == nil || len(m) != 0 {
		t.Errorf("ToMap(nil) = %#v", m)
	}
}
