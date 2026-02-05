package yamlish

import (
	"strings"
	"testing"
)

func TestFormatShapes(t *testing.T) {
	cases := []struct {
		in   any
		opts Options
		want string
	}{
		{map[string]any{"a": 1, "b": "x"}, Options{}, "{a: 1, b: x}"},
		{[]any{"x", "y"}, Options{NeverInlineArrays: true}, "- x\n- y"},
		{struct{ Name string }{"n"}, Options{}, "{Name: n}"},
	}
	for _, c := range cases {
		if got := Format(c.in, c.opts); got != c.want {
			t.Errorf("Format(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatHonoursBudget(t *testing.T) {
	items := make([]any, 500)
	for i := range items {
		items[i] = map[string]any{"id": i, "text": strings.Repeat("word ", 20)}
	}
	if got := Format(items, Options{MaxTotalLen: 2000}); len(got) > 2100 {
		t.Errorf("len = %d, budget 2000", len(got))
	}
}

func TestColorKeys(t *testing.T) {
	if got := Format(map[string]any{"k": 1}, Options{UseColor: true}); !strings.Contains(got, colorKey+"k"+colorReset) {
		t.Errorf("got %q", got)
	}
}
