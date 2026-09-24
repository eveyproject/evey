package syncplay_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/syncplay"
)

// The code is the ONLY secret protecting an unauthenticated guest token, and
// the route that trades it is public. 5 digits was 90 000 values: a per-IP
// limiter of 10/min holds one source off for ~75 h, but a thousand rotating
// sources walk the whole space in nine minutes. 6 symbols over a 32-letter
// alphabet is 2^30, which no rotation budget reaches.
func TestGenerateCode(t *testing.T) {
	t.Parallel()
	code, err := syncplay.GenerateCode()
	if err != nil {
		t.Fatalf("GenerateCode() error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("code length = %d, want 6", len(code))
	}
	for _, c := range code {
		// Crockford-style alphabet: no I, L, O or U, so nothing a guest reads
		// off a screen can be mistaken for a digit (and no accidental words).
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			t.Errorf("unexpected char %c in code %q", c, code)
		}
	}
}

func TestGenerateCode_Unique(t *testing.T) {
	t.Parallel()
	// Catches a stuck RNG, not a statistical collision: over 2^30 values a
	// repeat in 100 draws is impossible in practice, so any repeat is a bug.
	seen := make(map[string]int)
	const iters = 100
	for range iters {
		code, err := syncplay.GenerateCode()
		if err != nil {
			t.Fatalf("GenerateCode() error: %v", err)
		}
		seen[code]++
	}
	if len(seen) != iters {
		t.Fatalf("only %d unique codes in %d iterations - RNG is stuck", len(seen), iters)
	}
}

func TestNormalizeCode(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"abc123", "ABC123"},
		{" abc 123 ", "ABC123"},
		{"ABC-123", "ABC123"},
		// I/L read as 1 and O as 0: the alphabet excludes them, so folding a
		// misread character can never land on a different valid code.
		{"IL0O1", "11001"},
		// Old 5-digit rooms predate the alphabet and must keep working.
		{"12345", "12345"},
	}
	for _, tc := range tests {
		if got := syncplay.NormalizeCode(tc.in); got != tc.want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", tc.in, got, tc.want)
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

// A player reports currentTime * 1000, a float: decoding it must not drop the
// command (the websocket loop skipped a message it could not decode).
func TestSyncCommand_AcceptsAFractionalPosition(t *testing.T) {
	var c syncplay.SyncCommand
	if err := json.Unmarshal([]byte(`{"type":"pause","positionMs":67833.6}`), &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c.Type != "pause" || c.PositionMs != 67834 {
		t.Fatalf("got %+v, want pause at 67834", c)
	}
	var whole syncplay.SyncCommand
	if err := json.Unmarshal([]byte(`{"type":"seek","positionMs":1200,"trackIndex":2}`), &whole); err != nil || whole.PositionMs != 1200 || whole.TrackIndex != 2 {
		t.Fatalf("whole position: %+v %v", whole, err)
	}
}

// A position no title can have is refused, not converted: a float beyond int64
// has no defined conversion, and the value would be broadcast to the room.
func TestSyncCommand_RefusesAPositionOutOfRange(t *testing.T) {
	for _, raw := range []string{`{"type":"seek","positionMs":1e300}`, `{"type":"seek","positionMs":-5}`} {
		var c syncplay.SyncCommand
		if err := json.Unmarshal([]byte(raw), &c); err == nil {
			t.Fatalf("%s decoded to %+v, want an error", raw, c)
		}
	}
}
