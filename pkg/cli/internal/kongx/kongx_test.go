package kongx

import "testing"

func TestQuietRequested(t *testing.T) {
	for args, want := range map[string]bool{
		"--stdout-format=quiet":    true,
		"--stdout-verbosity 0":     true,
		"--stdout-format=json":     false,
		"-- --stdout-format=quiet": false,
	} {
		if got := QuietRequested(split(args)); got != want {
			t.Errorf("%q: %v", args, got)
		}
	}
}

func split(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}
