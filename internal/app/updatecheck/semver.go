package updatecheck

import (
	"strconv"
	"strings"
)

// semver is the subset the CI tags use: [v]MAJOR.MINOR.PATCH[-PRERELEASE].
type semver struct {
	major, minor, patch int
	pre                 string
}

func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(s, "v")
	core, pre, _ := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return semver{}, false
		}
		n[i] = v
	}
	return semver{major: n[0], minor: n[1], patch: n[2], pre: pre}, true
}

// less orders by semver precedence: a pre-release sorts before its release,
// pre-release identifiers compare numerically when both are numbers.
func (a semver) less(b semver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	if a.patch != b.patch {
		return a.patch < b.patch
	}
	switch {
	case a.pre == b.pre:
		return false
	case a.pre == "":
		return false
	case b.pre == "":
		return true
	}
	ap, bp := strings.Split(a.pre, "."), strings.Split(b.pre, ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		if ap[i] == bp[i] {
			continue
		}
		ai, aerr := strconv.Atoi(ap[i])
		bi, berr := strconv.Atoi(bp[i])
		switch {
		case aerr == nil && berr == nil:
			return ai < bi
		case aerr == nil:
			return true
		case berr == nil:
			return false
		default:
			return ap[i] < bp[i]
		}
	}
	return len(ap) < len(bp)
}
