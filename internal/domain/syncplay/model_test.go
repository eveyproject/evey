package syncplay_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
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
	// flake. Instead assert near-full uniqueness - catches a stuck
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
		t.Fatalf("only %d unique codes in %d iterations - RNG is stuck", len(seen), iters)
	}
	for code, count := range seen {
		if count > 2 {
			t.Fatalf("code %q appeared %d times - RNG is stuck", code, count)
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

func TestNewRoom_ValidInput(t *testing.T) {
	t.Parallel()
	before := time.Now().UTC()
	r, err := syncplay.NewRoom("room-1", "12345", "movie night", shared.UserID("u1"), shared.MediaItemID("mi-1"), "video")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("NewRoom() unexpected error: %v", err)
	}
	if r.ID != "room-1" {
		t.Errorf("ID = %q, want %q", r.ID, "room-1")
	}
	if r.MediaType != "video" {
		t.Errorf("MediaType = %q, want \"video\"", r.MediaType)
	}
	if r.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if r.CreatedAt.Before(before) || r.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not within [%v, %v]", r.CreatedAt, before, after)
	}
	if r.State.UpdatedAt.IsZero() {
		t.Error("State.UpdatedAt should be set")
	}
}

func TestNewRoom_DefaultsMediaType(t *testing.T) {
	t.Parallel()
	r, err := syncplay.NewRoom("room-2", "12345", "music room", shared.UserID("u1"), shared.MediaItemID("mi-2"), "")
	if err != nil {
		t.Fatalf("NewRoom() unexpected error: %v", err)
	}
	if r.MediaType != "video" {
		t.Errorf("MediaType = %q, want \"video\" (default)", r.MediaType)
	}
}

func TestNewRoom_RejectsEmptyCode(t *testing.T) {
	t.Parallel()
	_, err := syncplay.NewRoom("room-3", "", "name", shared.UserID("u1"), shared.MediaItemID("mi-1"), "video")
	if err == nil {
		t.Fatal("NewRoom() should reject empty Code")
	}
}

func TestNewRoom_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	_, err := syncplay.NewRoom("room-4", "12345", "", shared.UserID("u1"), shared.MediaItemID("mi-1"), "video")
	if err == nil {
		t.Fatal("NewRoom() should reject empty Name")
	}
}

func TestNewRoom_RejectsEmptyHostID(t *testing.T) {
	t.Parallel()
	_, err := syncplay.NewRoom("room-5", "12345", "name", shared.UserID(""), shared.MediaItemID("mi-1"), "video")
	if err == nil {
		t.Fatal("NewRoom() should reject empty HostID")
	}
}

func TestNewRoom_RejectsEmptyMediaID(t *testing.T) {
	t.Parallel()
	_, err := syncplay.NewRoom("room-6", "12345", "name", shared.UserID("u1"), shared.MediaItemID(""), "video")
	if err == nil {
		t.Fatal("NewRoom() should reject empty MediaID")
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
