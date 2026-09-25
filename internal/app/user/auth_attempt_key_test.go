package user

import "testing"

// The lockout key follows the lookup: case-insensitive and trimmed, so a
// locked "alice" does not get five fresh tries as "Alice" (or "Alice ").
func TestLoginAttemptKey_CaseAndSpaceInsensitive(t *testing.T) {
	for _, in := range []string{"Alice", "alice", " ALICE ", "aLiCe"} {
		if got := loginAttemptKey(in); got != "alice" {
			t.Fatalf("loginAttemptKey(%q) = %q, want alice", in, got)
		}
	}
}
