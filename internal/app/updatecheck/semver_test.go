package updatecheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Semver precedence decides whether the admin is told an update exists, so
// each rule is pinned on its own: the ordered-list test only walked adjacent
// pairs and left the major, equal-version and mixed-identifier branches
// untested (mutation 2026-09-28).
func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		less bool
	}{
		{"1.9.9", "2.0.0", true}, // major wins over everything below it
		{"2.0.0", "1.99.99", false},
		{"1.2.3", "1.2.3", false}, // equal is not less, either way
		{"1.2.3-beta.1", "1.2.3-beta.1", false},
		{"1.2.3", "1.2.3-rc.1", false}, // a release is never below its pre-release
		{"1.2.3-rc.1", "1.2.3", true},
		{"1.0.0-2", "1.0.0-alpha", true}, // numeric identifiers sort before alphanumeric
		{"1.0.0-alpha", "1.0.0-2", false},
		{"1.0.0-alpha", "1.0.0-beta", true},     // alphanumeric compare lexically
		{"1.0.0-alpha.1", "1.0.0-alpha", false}, // more identifiers sort after, not before
		{"v1.0.0", "1.0.1", true},               // the v prefix is ignored
	}
	for _, c := range cases {
		a, okA := parseSemver(c.a)
		b, okB := parseSemver(c.b)
		if !okA || !okB {
			t.Fatalf("fixtures must parse: %q %v, %q %v", c.a, okA, c.b, okB)
		}
		assert.Equal(t, c.less, a.less(b), "%s < %s", c.a, c.b)
	}
	for _, bad := range []string{"1.2", "1.2.3.4", "1.x.3", "1.-2.3", ""} {
		_, ok := parseSemver(bad)
		assert.False(t, ok, "%q is not a version", bad)
	}
}
