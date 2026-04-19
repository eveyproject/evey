package syncplay_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/syncplay"
)

func TestGenerateCode(t *testing.T) {
	t.Parallel()
	code, err := syncplay.GenerateCode()
	if err != nil {
		t.Fatalf("GenerateCode() error: %v", err)
	}
	if len(code) != 5 {
		t.Errorf("code length = %d, want 5", len(code))
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Errorf("unexpected char %c in code %q, want digits only", c, code)
		}
	}
}

func TestGenerateCode_Unique(t *testing.T) {
	t.Parallel()
	// Code space is 90000 (10000..99999). Birthday paradox at N=100
	// gives ~5% collision rate, so asserting zero collisions is a
	// flake. Instead assert near-full uniqueness — catches a stuck
	// RNG that returns the same value repeatedly without making the
	// test fail on a legitimate statistical collision.
	seen := make(map[string]int)
	const iters = 100
	for range iters {
		code, err := syncplay.GenerateCode()
		if err != nil {
			t.Fatalf("GenerateCode() error: %v", err)
		}
		seen[code]++
	}
	if len(seen) < iters-3 {
		t.Fatalf("only %d unique codes in %d iterations — RNG is stuck", len(seen), iters)
	}
	for code, count := range seen {
		if count > 2 {
			t.Fatalf("code %q appeared %d times — RNG is stuck", code, count)
		}
	}
}

func TestMaskUsername(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input, want string
	}{
		{"alice", "a***"},
		{"Bob", "B***"},
		{"", "***"},
		{"x", "x***"},
	}
	for _, tt := range tests {
		if got := syncplay.MaskUsername(tt.input); got != tt.want {
			t.Errorf("MaskUsername(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRoom_IsHost(t *testing.T) {
	t.Parallel()
	r := &syncplay.Room{HostID: "u1"}
	if !r.IsHost("u1") {
		t.Error("host should be recognized")
	}
	if r.IsHost("u2") {
		t.Error("non-host should not be recognized")
	}
}

func TestRoom_IsMember(t *testing.T) {
	t.Parallel()
	r := &syncplay.Room{
		HostID:  "u1",
		Members: []syncplay.Member{{UserID: "u2"}, {UserID: "u3"}},
	}

	tests := []struct {
		userID string
		want   bool
	}{
		{"u1", true},  // host
		{"u2", true},  // member
		{"u3", true},  // member
		{"u4", false}, // stranger
	}
	for _, tt := range tests {
		if got := r.IsMember(tt.userID); got != tt.want {
			t.Errorf("IsMember(%q) = %v, want %v", tt.userID, got, tt.want)
		}
	}
}
