package syncplay_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/syncplay"
)

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
