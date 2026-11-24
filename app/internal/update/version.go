package update

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed semantic version.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
	Valid               bool
}

// ParseVersion parses "v1.2.3" or "1.2.3-beta.1". Anything else (for example "dev") is invalid.
func ParseVersion(s string) Version {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, pre, _ := strings.Cut(s, "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return Version{}
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2], pre, true}
}

// Compare returns -1, 0 or 1. Invalid versions sort below every valid one; a pre-release is below its release.
func Compare(a, b Version) int {
	switch {
	case !a.Valid && !b.Valid:
		return 0
	case !a.Valid:
		return -1
	case !b.Valid:
		return 1
	}
	for _, p := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	return comparePre(a.Pre, b.Pre)
}

// comparePre orders two pre-release labels as semantic versioning does: the dot-separated parts one by one, a number
// by its value (beta.9 is below beta.11), a number below a word, a word by its letters, and the label with fewer
// parts below one that begins the same way.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		xn, xerr := strconv.ParseUint(x, 10, 64)
		yn, yerr := strconv.ParseUint(y, 10, 64)
		switch {
		case xerr == nil && yerr == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case xerr == nil:
			return -1
		case yerr == nil:
			return 1
		case x != y:
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

// Newer reports whether candidate is a higher version than current.
func Newer(candidate, current string) bool {
	return Compare(ParseVersion(candidate), ParseVersion(current)) > 0
}

// String renders v.
func (v Version) String() string {
	if !v.Valid {
		return "unknown"
	}
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}
