// Package semver compares semantic versions. It is internal and
// stdlib-only so version ordering does not depend on which semver
// library a release source happens to use.
package semver

import (
	"strconv"
	"strings"
)

// Version is a parsed semantic version. Build metadata is kept for
// display but ignored by Compare, as SemVer requires.
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string
	Build               string
	raw                 string
}

// Parse accepts "1.2.3", "v1.2.3" and "1.2.3-rc.1+meta". Missing minor
// or patch components default to zero so tags like "v2" still order.
func Parse(s string) (Version, bool) {
	raw := s
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return Version{}, false
	}
	v := Version{raw: raw}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		v.Build = s[i+1:]
		s = s[:i]
		if !validIdents(v.Build) {
			return Version{}, false
		}
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		if !validIdents(pre) {
			return Version{}, false
		}
		v.Pre = strings.Split(pre, ".")
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return Version{}, false
	}
	var nums [3]uint64
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return Version{}, false
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, true
}

// validIdents reports whether list is a non-empty, dot-separated run of
// non-empty identifiers, as SemVer 2.0.0 requires for both prerelease
// and build metadata.
func validIdents(list string) bool {
	for _, id := range strings.Split(list, ".") {
		if id == "" {
			return false
		}
	}
	return true
}

// String returns the original spelling.
func (v Version) String() string { return v.raw }

// Prerelease reports whether the version carries a prerelease tag.
func (v Version) Prerelease() bool { return len(v.Pre) > 0 }

// Compare returns -1, 0 or +1 following SemVer 2.0.0 precedence.
func Compare(a, b Version) int {
	for _, d := range [3][2]uint64{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}
	for i := 0; i < len(a.Pre) && i < len(b.Pre); i++ {
		if c := compareIdent(a.Pre[i], b.Pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a.Pre) < len(b.Pre):
		return -1
	case len(a.Pre) > len(b.Pre):
		return 1
	}
	return 0
}

// CompareStrings parses both sides. An unparsable version sorts below
// any parsable one so a malformed tag never wins "latest".
func CompareStrings(a, b string) int {
	va, okA := Parse(a)
	vb, okB := Parse(b)
	switch {
	case !okA && !okB:
		return strings.Compare(a, b)
	case !okA:
		return -1
	case !okB:
		return 1
	}
	return Compare(va, vb)
}

func compareIdent(a, b string) int {
	na, errA := strconv.ParseUint(a, 10, 64)
	nb, errB := strconv.ParseUint(b, 10, 64)
	switch {
	case errA == nil && errB == nil:
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		}
		return 0
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}
