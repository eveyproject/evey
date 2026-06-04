package ratings

import "testing"

func hasRating(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

// AllowedRatingsForAge maps a profile's max age to the set of content ratings
// it may see. A regression here is a parental-controls failure (a kid sees
// adult content), so lock the security-relevant invariants.
func TestAllowedRatingsForAge(t *testing.T) {
	if got := AllowedRatingsForAge(-1); got != nil {
		t.Errorf("negative age (parental disabled) must return nil, got %v", got)
	}

	age0 := AllowedRatingsForAge(0)
	for _, r := range []string{"G", "U"} {
		if !hasRating(age0, r) {
			t.Errorf("age 0 must allow the all-ages rating %q, got %v", r, age0)
		}
	}
	for _, r := range []string{"PG-13", "R", "18"} {
		if hasRating(age0, r) {
			t.Errorf("age 0 must NOT allow %q", r)
		}
	}

	age17 := AllowedRatingsForAge(17)
	for _, r := range []string{"R", "TV-MA"} {
		if !hasRating(age17, r) {
			t.Errorf("age 17 must allow %q", r)
		}
	}
	for _, r := range []string{"18", "X"} {
		if hasRating(age17, r) {
			t.Errorf("age 17 must NOT allow the 18+ rating %q", r)
		}
	}

	age18 := AllowedRatingsForAge(18)
	for _, r := range []string{"18", "X"} {
		if !hasRating(age18, r) {
			t.Errorf("age 18 must allow %q", r)
		}
	}

	// Unrated (NR/UR, min-age -1) must NEVER be auto-allowed, even at a very high
	// max age — fail-closed: an unclassified item is not silently kid-safe.
	high := AllowedRatingsForAge(99)
	for _, r := range []string{"NR", "UR"} {
		if hasRating(high, r) {
			t.Errorf("unrated %q (min-age -1) must never be allowed", r)
		}
	}

	// Monotonic: a higher max age allows a superset of a lower one.
	young := AllowedRatingsForAge(10)
	for _, r := range young {
		if !hasRating(age18, r) {
			t.Errorf("rating %q allowed at age 10 must still be allowed at 18", r)
		}
	}
	if len(age18) <= len(young) {
		t.Errorf("age 18 must allow strictly more than age 10 (%d vs %d)", len(age18), len(young))
	}

	// Deduplicated: ratings shared across country tables appear once.
	seen := map[string]int{}
	for _, r := range age18 {
		seen[r]++
	}
	for r, n := range seen {
		if n != 1 {
			t.Errorf("rating %q appears %d times, want 1 (dedup)", r, n)
		}
	}
}
