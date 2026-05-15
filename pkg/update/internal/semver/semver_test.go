package semver

import "testing"

func TestParse(t *testing.T) {
	valid := []string{
		"1.2.3", "v1.2.3", "v2", "1.2.3-rc.1", "1.2.3-rc.1+meta",
		"1.0.0+a.b", "1.0.0-alpha", "1.0.0-x.7.z.92",
	}
	for _, s := range valid {
		if _, ok := Parse(s); !ok {
			t.Errorf("Parse(%q) rejected", s)
		}
	}
	// SemVer 2.0.0 forbids empty prerelease and build identifiers.
	invalid := []string{
		"", "1.2.x", "1.0.0-", "1.0.0-rc.", "1.0.0-.rc",
		"1.0.0+", "1.0.0+a.", "1.0.0-rc.+b", "1.0.0-rc.1+",
	}
	for _, s := range invalid {
		if _, ok := Parse(s); ok {
			t.Errorf("Parse(%q) accepted", s)
		}
	}
}

func TestCompare(t *testing.T) {
	if CompareStrings("1.0.0-rc.1", "1.0.0") != -1 || CompareStrings("1.0.0", "v1.0.0") != 0 {
		t.Error("prerelease or equivalence ordering")
	}
	if CompareStrings("1.0.0-rc.1", "1.0.0-rc.2") != -1 || CompareStrings("1.0.0-rc.1", "1.0.0-rc.1+b") != 0 {
		t.Error("prerelease or build-metadata ordering")
	}
	if CompareStrings("garbage", "1.0.0") != -1 {
		t.Error("unparsable must sort below parsable")
	}
}
