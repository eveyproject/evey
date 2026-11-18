package plugin

import (
	"strings"
	"testing"
)

// The owner and the repository of a followed link are held to a small
// alphabet: each end of every range it allows, and the character just
// outside it.
func TestValidRepoSegment(t *testing.T) {
	cases := []struct {
		segment string
		valid   bool
	}{
		{"", false},
		{".", false},
		{"..", false},
		{"...", true},
		{"a", true}, {"z", true}, {"A", true}, {"Z", true}, {"0", true}, {"9", true},
		{"-", true}, {"_", true}, {"x.y", true},
		{"`", false}, {"{", false},
		{"@", false}, {"[", false},
		{"/", false}, {":", false},
		{"a b", false}, {"a/b", false}, {"é", false}, {"a\x00", false},
		{"Evey-plugin_1.0", true},
		{strings.Repeat("a", 100), true},
		{strings.Repeat("a", 101), false},
	}
	for _, c := range cases {
		if got := validRepoSegment(c.segment); got != c.valid {
			t.Errorf("validRepoSegment(%q) = %v, want %v", c.segment, got, c.valid)
		}
	}
}
