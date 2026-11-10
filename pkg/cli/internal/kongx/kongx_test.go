package kongx

import "testing"

func TestSilentRequested(t *testing.T) {
	for _, c := range []struct {
		args, env string
		want      bool
	}{
		{"--stdout-verbosity 0", "", true},
		{"--stdout-verbosity=Silent", "", true},
		{"--stdout-verbosity=quiet", "", false},
		{"--stdout-format=quiet", "", false},
		{"-- --stdout-verbosity=silent", "", false},
		{"show", "silent", true},
		{"show", "0", true},
		{"show", "quiet", false},
		{"--stdout-verbosity=warn", "silent", false},
		{"--stdout-verbosity=warn --stdout-verbosity=silent", "", true},
	} {
		if got := SilentRequested(split(c.args), c.env); got != c.want {
			t.Errorf("%q env %q: %v", c.args, c.env, got)
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
