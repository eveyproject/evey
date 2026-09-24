package syncplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/syncplay"
	"github.com/evey-project/evey/internal/domain/syncplay/fakes"
)

// captureSlog swaps the default slog logger for the duration of the test
// and returns a buffer with emitted log output. Not parallel-safe.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// fakeRoomRepo is an in-memory implementation of syncplay.RoomRepository.
type fakeRoomRepo struct {
	mu    sync.Mutex
	rooms map[string]*syncplay.Room
}

func newFakeRoomRepo() *fakeRoomRepo {
	return &fakeRoomRepo{rooms: make(map[string]*syncplay.Room)}
}

func (r *fakeRoomRepo) Create(_ context.Context, room *syncplay.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[room.ID] = room
	return nil
}

func (r *fakeRoomRepo) Get(_ context.Context, id string) (*syncplay.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[id]
	if !ok {
		return nil, errors.New("room not found")
	}
	cp := *room
	return &cp, nil
}

func (r *fakeRoomRepo) GetByCode(_ context.Context, code string) (*syncplay.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, room := range r.rooms {
		if room.Code == code {
			cp := *room
			return &cp, nil
		}
	}
	return nil, errors.New("room not found")
}

func (r *fakeRoomRepo) AddMember(_ context.Context, roomID string, member syncplay.Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return errors.New("room not found")
	}
	for _, m := range room.Members {
		if m.UserID == member.UserID {
			return nil
		}
	}
	room.Members = append(room.Members, member)
	return nil
}

func (r *fakeRoomRepo) RemoveMember(_ context.Context, roomID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return errors.New("room not found")
	}
	filtered := room.Members[:0]
	for _, m := range room.Members {
		if string(m.UserID) != userID {
			filtered = append(filtered, m)
		}
	}
	room.Members = filtered
	return nil
}

func (r *fakeRoomRepo) RemoveMemberAndDeleteIfEmpty(ctx context.Context, roomID, userID string) (bool, string, error) {
	// Like the postgres transaction, a cancelled context does nothing.
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return false, "", errors.New("room not found")
	}
	filtered := make([]syncplay.Member, 0, len(room.Members))
	for _, m := range room.Members {
		if string(m.UserID) != userID {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		delete(r.rooms, roomID)
		return true, "", nil
	}
	// Elect a new host if the departing member was the host - mirrors the
	// atomicity guarantee provided by the postgres FOR UPDATE transaction.
	newHostID := ""
	if string(room.HostID) == userID {
		newHost := syncplay.OldestMember(filtered)
		newHostID = string(newHost.UserID)
		room.HostID = newHost.UserID
	}
	room.Members = filtered
	return false, newHostID, nil
}

func (r *fakeRoomRepo) UpdateState(_ context.Context, roomID string, state syncplay.PlaybackState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return errors.New("room not found")
	}
	room.State = state
	return nil
}

func (r *fakeRoomRepo) UpdateTrackIndex(_ context.Context, roomID string, idx int, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return errors.New("room not found")
	}
	room.TrackIndex = idx
	room.MediaID = shared.MediaItemID(mediaID)
	return nil
}

func (r *fakeRoomRepo) AdvanceTrackIndex(_ context.Context, roomID string, delta int) (int, string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return 0, "", false, errors.New("room not found")
	}
	newIdx := room.TrackIndex + delta
	if newIdx < 0 || newIdx >= len(room.Queue) {
		return 0, "", false, nil
	}
	room.TrackIndex = newIdx
	room.MediaID = room.Queue[newIdx]
	return newIdx, string(room.MediaID), true, nil
}

func (r *fakeRoomRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, id)
	return nil
}

func (r *fakeRoomRepo) DeleteStaleEmptyRooms(_ context.Context, olderThan time.Time) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for id, room := range r.rooms {
		if len(room.Members) == 0 && room.CreatedAt.Before(olderThan) {
			delete(r.rooms, id)
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (r *fakeRoomRepo) CountActiveByHost(_ context.Context, hostID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, room := range r.rooms {
		if string(room.HostID) == hostID {
			n++
		}
	}
	return n, nil
}

// TestHub_CreateRoom_PerUserCap proves a single user cannot open unbounded
// rooms. The janitor only bounds growth in TIME (empty rooms age out); a user
// scripting CreateRoom in a tight loop could still spike the table between
// sweeps. The cap refuses past the ceiling with a typed error.
func TestHub_CreateRoom_PerUserCap(t *testing.T) {
	t.Parallel()
	h := NewHub(newFakeRoomRepo())
	h.maxRoomsPerHost = 3
	ctx := context.Background()

	for i := range 3 {
		if _, err := h.CreateRoom(ctx, "host-1", "m1", "r"); err != nil {
			t.Fatalf("room %d should be allowed: %v", i, err)
		}
	}
	_, err := h.CreateRoom(ctx, "host-1", "m1", "r")
	if !errors.Is(err, ErrTooManyRooms) {
		t.Fatalf("the 4th room must be refused with ErrTooManyRooms, got %v", err)
	}
	// A DIFFERENT host is unaffected: the cap is per-user.
	if _, err := h.CreateRoom(ctx, "host-2", "m1", "r"); err != nil {
		t.Fatalf("another host must still create: %v", err)
	}
}

// The cap covers EVERY creation entry point, not just the video one - a music
// room from a playlist counts against the same ceiling.
func TestHub_CreateRoomFromQueue_RespectsCap(t *testing.T) {
	t.Parallel()
	h := NewHub(newFakeRoomRepo())
	h.maxRoomsPerHost = 1
	ctx := context.Background()

	if _, err := h.CreateRoom(ctx, "host-1", "m1", "r"); err != nil {
		t.Fatal(err)
	}
	_, err := h.CreateRoomFromQueue(ctx, "host-1", "r", []string{"t1"})
	if !errors.Is(err, ErrTooManyRooms) {
		t.Fatalf("queue room past the cap must be refused, got %v", err)
	}
}

func TestHub_CreateRoomFromPlaylist_RespectsCap(t *testing.T) {
	t.Parallel()
	h := NewHub(newFakeRoomRepo())
	h.maxRoomsPerHost = 1
	ctx := context.Background()

	if _, err := h.CreateRoom(ctx, "host-1", "m1", "r"); err != nil {
		t.Fatal(err)
	}
	_, err := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "r", []string{"t1"})
	if !errors.Is(err, ErrTooManyRooms) {
		t.Fatalf("playlist room past the cap must be refused, got %v", err)
	}
}

func newTestHub() *Hub {
	return NewHub(newFakeRoomRepo())
}

func TestHub_CreateRoom_HasCode(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, err := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if len(room.Code) != 6 {
		t.Errorf("Code length = %d, want 6", len(room.Code))
	}
}

func TestHub_GetRoomByCode(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, err := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	found, err := h.GetRoomByCode(ctx, room.Code)
	if err != nil {
		t.Fatalf("GetRoomByCode: %v", err)
	}
	if found.ID != room.ID {
		t.Errorf("ID = %q, want %q", found.ID, room.ID)
	}
}

func TestHub_GetRoomByCode_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	_, err := h.GetRoomByCode(ctx, "99999")
	if err == nil {
		t.Error("expected error for nonexistent code")
	}
}

func TestHub_CreateRoom(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, err := h.CreateRoom(ctx, "host-1", "media-1", "Movie Night")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.ID == "" {
		t.Error("room ID should not be empty")
	}
	if room.HostID != "host-1" {
		t.Errorf("HostID = %q, want %q", room.HostID, "host-1")
	}
	if room.MediaID != "media-1" {
		t.Errorf("MediaID = %q, want %q", room.MediaID, "media-1")
	}
	if room.Name != "Movie Night" {
		t.Errorf("Name = %q, want %q", room.Name, "Movie Night")
	}
}

func TestHub_CreateRoom_Persisted(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	created, err := h.CreateRoom(ctx, "host-1", "media-1", "Test Room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	fetched, err := h.GetRoom(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", fetched.ID, created.ID)
	}
}

func TestHub_GetRoom_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	_, err := h.GetRoom(ctx, "nonexistent-room")
	if err == nil {
		t.Error("expected error for nonexistent room")
	}
}

func TestHub_JoinRoom_Success(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, err := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if joinErr := h.JoinRoom(ctx, room.ID, "user-2", "Alice"); joinErr != nil {
		t.Fatalf("JoinRoom: %v", joinErr)
	}

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	found := false
	for _, m := range fetched.Members {
		if m.UserID == "user-2" && m.Username == "Alice" {
			found = true
			break
		}
	}
	if !found {
		t.Error("user-2 not found in room members after join")
	}
}

func TestHub_JoinRoom_NonexistentRoom(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	err := h.JoinRoom(ctx, "bad-room-id", "user-1", "Bob")
	if err == nil {
		t.Error("expected error joining nonexistent room")
	}
}

func TestHub_LeaveRoom_RemovesMember(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "host-1", "Host")
	_ = h.JoinRoom(ctx, room.ID, "user-2", "Alice")

	if err := h.LeaveRoom(ctx, room.ID, "user-2"); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	for _, m := range fetched.Members {
		if m.UserID == "user-2" {
			t.Error("user-2 should not be in room after leave")
		}
	}
}

func TestHub_LeaveRoom_DeletesRoomWhenEmpty(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "host-1", "Host")
	_ = h.LeaveRoom(ctx, room.ID, "host-1")

	_, err := h.GetRoom(ctx, room.ID)
	if err == nil {
		t.Error("room should be deleted when last member leaves")
	}
}

// A guest joined by code holds a token that names no user, so nothing else can
// ever reach it: ending the party has to be what takes it back. Otherwise the
// host can close the room and the guest keeps streaming the title for hours.
type recordingRevoker struct {
	mu    sync.Mutex
	rooms []string
}

func (r *recordingRevoker) RevokeByRoom(roomID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms = append(r.rooms, roomID)
	return 0
}

func (r *recordingRevoker) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.rooms...)
}

func TestHub_LeaveRoom_RevokesGuestTokensWhenRoomDies(t *testing.T) {
	t.Parallel()
	rev := &recordingRevoker{}
	h := NewHub(newFakeRoomRepo(), WithGuestTokenRevoker(rev))
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "host-1", "Host")
	_ = h.JoinRoom(ctx, room.ID, "user-2", "Guest")

	// Someone leaving is not the party ending: the room lives on, and so must
	// the tokens of everyone still watching.
	_ = h.LeaveRoom(ctx, room.ID, "user-2")
	if got := rev.seen(); len(got) != 0 {
		t.Fatalf("revoked %v while the room was still alive", got)
	}

	_ = h.LeaveRoom(ctx, room.ID, "host-1")
	if got := rev.seen(); len(got) != 1 || got[0] != room.ID {
		t.Fatalf("revoked rooms = %v, want [%s] once the room was deleted", got, room.ID)
	}
}

func TestHub_Janitor_RevokesGuestTokensOfSweptRooms(t *testing.T) {
	t.Parallel()
	rev := &recordingRevoker{}
	repo := newFakeRoomRepo()
	h := NewHub(repo, WithGuestTokenRevoker(rev))
	ctx := context.Background()

	// A room whose host never opened the WebSocket: it never goes through
	// LeaveRoom, so the janitor is the only thing that ever deletes it - and
	// its code may already have minted a guest token.
	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")

	ctx, cancel := context.WithCancel(ctx)
	cancel()
	h.StartJanitor(ctx, time.Hour, -time.Second)

	if got := rev.seen(); len(got) != 1 || got[0] != room.ID {
		t.Fatalf("revoked rooms = %v, want [%s] after the sweep", got, room.ID)
	}
}

func TestHub_MultipleRooms_Isolated(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room1, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room 1")
	room2, _ := h.CreateRoom(ctx, "host-2", "media-2", "Room 2")

	if room1.ID == room2.ID {
		t.Error("two rooms should have distinct IDs")
	}

	_ = h.JoinRoom(ctx, room1.ID, "user-A", "Alice")
	_ = h.JoinRoom(ctx, room2.ID, "user-B", "Bob")

	fetched1, _ := h.GetRoom(ctx, room1.ID)
	fetched2, _ := h.GetRoom(ctx, room2.ID)

	for _, m := range fetched1.Members {
		if m.UserID == "user-B" {
			t.Error("user-B should not appear in room1")
		}
	}
	for _, m := range fetched2.Members {
		if m.UserID == "user-A" {
			t.Error("user-A should not appear in room2")
		}
	}
}

func TestHub_GetRoom_ReturnsMemberList(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "host-1", "Host")
	_ = h.JoinRoom(ctx, room.ID, "user-2", "Alice")
	_ = h.JoinRoom(ctx, room.ID, "user-3", "Bob")

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if len(fetched.Members) != 3 {
		t.Errorf("expected 3 members, got %d", len(fetched.Members))
	}
}

func TestHub_LeaveRoom_UpdatesMemberList(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "host-1", "Host")
	_ = h.JoinRoom(ctx, room.ID, "user-2", "Alice")
	_ = h.JoinRoom(ctx, room.ID, "user-3", "Bob")
	_ = h.LeaveRoom(ctx, room.ID, "user-2")

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if len(fetched.Members) != 2 {
		t.Errorf("expected 2 members after leave, got %d", len(fetched.Members))
	}
	for _, m := range fetched.Members {
		if m.UserID == "user-2" {
			t.Error("user-2 should not be in member list after leave")
		}
	}
}

func TestHub_UpdateState_Play(t *testing.T) {
	t.Parallel()
	repo := newFakeRoomRepo()
	h := NewHub(repo)
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")

	state := syncplay.PlaybackState{
		Playing:    true,
		PositionMs: 5000,
	}
	if err := repo.UpdateState(ctx, room.ID, state); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if !fetched.State.Playing {
		t.Error("expected Playing=true after play state update")
	}
	if fetched.State.PositionMs != 5000 {
		t.Errorf("expected PositionMs=5000, got %d", fetched.State.PositionMs)
	}
}

func TestHub_UpdateState_Pause(t *testing.T) {
	t.Parallel()
	repo := newFakeRoomRepo()
	h := NewHub(repo)
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")

	// First play
	_ = repo.UpdateState(ctx, room.ID, syncplay.PlaybackState{Playing: true, PositionMs: 10000})

	// Then pause
	_ = repo.UpdateState(ctx, room.ID, syncplay.PlaybackState{Playing: false, PositionMs: 10000})

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if fetched.State.Playing {
		t.Error("expected Playing=false after pause")
	}
}

func TestHub_UpdateState_Seek(t *testing.T) {
	t.Parallel()
	repo := newFakeRoomRepo()
	h := NewHub(repo)
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = repo.UpdateState(ctx, room.ID, syncplay.PlaybackState{Playing: true, PositionMs: 120000})

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if fetched.State.PositionMs != 120000 {
		t.Errorf("expected PositionMs=120000 after seek, got %d", fetched.State.PositionMs)
	}
	if !fetched.State.Playing {
		t.Error("playing state should remain true after seek")
	}
}

// TestExtractWSToken and TestHub_Broadcast_SkipsSender moved to adapters/driving/http/ (transport layer)

// --- HandleCommand tests ---

func TestHub_HandleCommand_Play(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")

	cmd, err := h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type:       "play",
		PositionMs: 5000,
	})
	if err != nil {
		t.Fatalf("HandleCommand play: %v", err)
	}
	if cmd.UserID != "host-1" {
		t.Errorf("cmd.UserID = %q, want %q", cmd.UserID, "host-1")
	}
	if cmd.Username != "A***" {
		t.Errorf("cmd.Username = %q, want %q", cmd.Username, "A***")
	}
	if cmd.RoomID != room.ID {
		t.Errorf("cmd.RoomID = %q, want %q", cmd.RoomID, room.ID)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if !fetched.State.Playing {
		t.Error("expected Playing=true after play command")
	}
	if fetched.State.PositionMs != 5000 {
		t.Errorf("PositionMs = %d, want 5000", fetched.State.PositionMs)
	}
	if fetched.State.UpdatedBy != "host-1" {
		t.Errorf("UpdatedBy = %q, want %q", fetched.State.UpdatedBy, "host-1")
	}
}

func TestHub_HandleCommand_Resync_AnyMember(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err := h.JoinRoom(ctx, room.ID, "guest-1", "Bob"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Unlike host-only play/pause/seek, a non-host member CAN pull the party to
	// their position (the v2 "everyone sync to me").
	if _, err := h.HandleCommand(ctx, room.ID, "guest-1", "Bob", syncplay.SyncCommand{
		Type:       "resync",
		PositionMs: 42000,
	}); err != nil {
		t.Fatalf("HandleCommand resync: %v", err)
	}
	fetched, _ := h.GetRoom(ctx, room.ID)
	if !fetched.State.Playing || fetched.State.PositionMs != 42000 || fetched.State.UpdatedBy != "guest-1" {
		t.Errorf("resync must move the room to the sender's position and play, got %+v", fetched.State)
	}

	// A non-member must not be able to resync the room.
	_, _ = h.HandleCommand(ctx, room.ID, "outsider", "Eve", syncplay.SyncCommand{
		Type:       "resync",
		PositionMs: 99000,
	})
	after, _ := h.GetRoom(ctx, room.ID)
	if after.State.PositionMs == 99000 {
		t.Error("a non-member must not be able to resync the room")
	}
}

func TestHub_HandleCommand_Pause(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	// Start playing
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type: "play", PositionMs: 1000,
	})

	cmd, err := h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type: "pause", PositionMs: 3000,
	})
	if err != nil {
		t.Fatalf("HandleCommand pause: %v", err)
	}
	if cmd.Type != "pause" {
		t.Errorf("cmd.Type = %q, want %q", cmd.Type, "pause")
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if fetched.State.Playing {
		t.Error("expected Playing=false after pause command")
	}
	if fetched.State.PositionMs != 3000 {
		t.Errorf("PositionMs = %d, want 3000", fetched.State.PositionMs)
	}
}

func TestHub_HandleCommand_Seek_PreservesPlaying(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	// Start playing
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type: "play", PositionMs: 1000,
	})

	_, err := h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type: "seek", PositionMs: 50000,
	})
	if err != nil {
		t.Fatalf("HandleCommand seek: %v", err)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if !fetched.State.Playing {
		t.Error("seek should preserve Playing=true")
	}
	if fetched.State.PositionMs != 50000 {
		t.Errorf("PositionMs = %d, want 50000", fetched.State.PositionMs)
	}
}

func TestHub_HandleCommand_Seek_PreservesPaused(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	// Room starts paused by default (Playing=false)

	_, err := h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type: "seek", PositionMs: 20000,
	})
	if err != nil {
		t.Fatalf("HandleCommand seek: %v", err)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if fetched.State.Playing {
		t.Error("seek should preserve Playing=false when paused")
	}
}

func TestHub_HandleCommand_UnknownType_NoStateChange(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")

	cmd, err := h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type: "unknown",
	})
	if err != nil {
		t.Fatalf("HandleCommand unknown: %v", err)
	}
	if cmd.RoomID != room.ID {
		t.Errorf("cmd.RoomID = %q, want %q", cmd.RoomID, room.ID)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if fetched.State.Playing {
		t.Error("unknown command should not change state")
	}
}

func TestHub_HandleCommand_NonHost_PlayRejected(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "guest-2", "Bob")

	// A guest sending play must NOT change room state (host-only control).
	_, err := h.HandleCommand(ctx, room.ID, "guest-2", "Bob", syncplay.SyncCommand{
		Type: "play", PositionMs: 99000,
	})
	if err != nil {
		t.Fatalf("HandleCommand (guest play) should be a silent no-op, got err: %v", err)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if fetched.State.Playing {
		t.Error("guest play must not start playback")
	}
	if fetched.State.PositionMs == 99000 {
		t.Error("guest play must not write its position to room state")
	}
}

func TestHub_HandleCommand_NonHost_SeekRejected(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "guest-2", "Bob")
	// Host sets a known position first.
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "Alice", syncplay.SyncCommand{
		Type: "seek", PositionMs: 10000,
	})

	// Guest seek must be ignored, leaving the host's position intact.
	_, err := h.HandleCommand(ctx, room.ID, "guest-2", "Bob", syncplay.SyncCommand{
		Type: "seek", PositionMs: 777000,
	})
	if err != nil {
		t.Fatalf("guest seek should be a silent no-op, got err: %v", err)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if fetched.State.PositionMs != 10000 {
		t.Errorf("guest seek must not move position; got %d want 10000", fetched.State.PositionMs)
	}
}

func TestHub_HandleCommand_NonexistentRoom(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	_, err := h.HandleCommand(ctx, "bad-room", "user-1", "Alice", syncplay.SyncCommand{
		Type: "play", PositionMs: 0,
	})
	if err == nil {
		t.Error("expected error for nonexistent room")
	}
}

// --- ConnectMember tests ---

func TestHub_ConnectMember_Success(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	sender := fakes.NewFakeMessageSender()

	got, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", sender)
	if err != nil {
		t.Fatalf("ConnectMember: %v", err)
	}
	if got.ID != room.ID {
		t.Errorf("returned room ID = %q, want %q", got.ID, room.ID)
	}

	found := false
	for _, m := range got.Members {
		if m.UserID == "user-2" && m.Username == "Alice" {
			found = true
		}
	}
	if !found {
		t.Error("user-2 not found in members after ConnectMember")
	}

	// Should have received state message
	msgs := sender.Received()
	if len(msgs) == 0 {
		t.Fatal("expected state message on connect")
	}
	var stateCmd syncplay.SyncCommand
	if err := json.Unmarshal(msgs[0], &stateCmd); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if stateCmd.Type != "state" {
		t.Errorf("first message type = %q, want %q", stateCmd.Type, "state")
	}
}

// A member joining a PLAYING room must seek to the live position (stored snapshot
// + time elapsed since it was taken), not the stale snapshot, or it starts behind.
func TestHub_ConnectMember_PlayingGivesLivePosition(t *testing.T) {
	t.Parallel()
	repo := newFakeRoomRepo()
	h := NewHub(repo)
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	// Host has been playing from 5000ms for ~10s.
	if err := repo.UpdateState(ctx, room.ID, syncplay.PlaybackState{
		Playing: true, PositionMs: 5000, UpdatedAt: time.Now().UTC().Add(-10 * time.Second),
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	sender := fakes.NewFakeMessageSender()
	if _, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", sender); err != nil {
		t.Fatalf("ConnectMember: %v", err)
	}
	var st syncplay.SyncCommand
	if err := json.Unmarshal(sender.Received()[0], &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !st.Playing {
		t.Error("expected Playing=true")
	}
	if st.PositionMs < 14000 {
		t.Errorf("joiner PositionMs = %d, want ~15000 (5000 + ~10s elapsed), not the stale 5000", st.PositionMs)
	}
}

// A member joining a PAUSED room gets the stored position unchanged (no advance).
func TestHub_ConnectMember_PausedKeepsStoredPosition(t *testing.T) {
	t.Parallel()
	repo := newFakeRoomRepo()
	h := NewHub(repo)
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err := repo.UpdateState(ctx, room.ID, syncplay.PlaybackState{
		Playing: false, PositionMs: 5000, UpdatedAt: time.Now().UTC().Add(-10 * time.Second),
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	sender := fakes.NewFakeMessageSender()
	if _, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", sender); err != nil {
		t.Fatalf("ConnectMember: %v", err)
	}
	var st syncplay.SyncCommand
	if err := json.Unmarshal(sender.Received()[0], &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if st.PositionMs != 5000 {
		t.Errorf("paused joiner PositionMs = %d, want 5000 (no advance while paused)", st.PositionMs)
	}
}

func TestHub_ConnectMember_Idempotent(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	s1 := fakes.NewFakeMessageSender()
	s2 := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", s1)
	got, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", s2)
	if err != nil {
		t.Fatalf("ConnectMember second call: %v", err)
	}

	count := 0
	for _, m := range got.Members {
		if m.UserID == "user-2" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 entry for user-2, got %d", count)
	}
}

func TestHub_ConnectMember_NonexistentRoom(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	_, err := h.ConnectMember(ctx, "bad-room", "user-1", "Alice", fakes.NewFakeMessageSender())
	if err == nil {
		t.Error("expected error connecting to nonexistent room")
	}
}

// getFailRepo delegates everything to a real fakeRoomRepo but forces Get to
// fail, exercising ConnectMember's rollback path (Get fails after the member
// and connection have already been registered).
type getFailRepo struct {
	*fakeRoomRepo
}

func (r *getFailRepo) Get(context.Context, string) (*syncplay.Room, error) {
	return nil, errors.New("transient repo failure")
}

func TestHub_ConnectMember_GetFailure_RollsBack(t *testing.T) {
	t.Parallel()
	base := newFakeRoomRepo()
	h := NewHub(&getFailRepo{base})
	ctx := context.Background()

	room, err := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if _, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", fakes.NewFakeMessageSender()); err == nil {
		t.Fatal("expected ConnectMember to fail when Get fails")
	}

	// Connection must not be left dangling in the hub.
	h.mu.RLock()
	leaked := len(h.conns[room.ID])
	h.mu.RUnlock()
	if leaked != 0 {
		t.Errorf("h.conns leaked %d entries for room after failed connect, want 0", leaked)
	}

	// The phantom room member must have been removed.
	stored, ok := base.rooms[room.ID]
	if !ok {
		t.Fatal("room missing from repo")
	}
	for _, m := range stored.Members {
		if string(m.UserID) == "user-2" {
			t.Error("user-2 still a room member after failed connect; rollback did not remove it")
		}
	}
}

// --- DisconnectMember tests ---

// The websocket's request context is already cancelled when its disconnect
// runs: the cleanup must still remove the member and elect a new host.
func TestHub_DisconnectMember_OutlivesTheClosedConnection(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	room, _ := h.CreateRoom(context.Background(), "host-1", "media-1", "Room")
	_, _ = h.ConnectMember(context.Background(), room.ID, "host-1", "Host", fakes.NewFakeMessageSender())
	_, _ = h.ConnectMember(context.Background(), room.ID, "user-2", "Alice", fakes.NewFakeMessageSender())

	closed, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.DisconnectMember(closed, room.ID, "host-1"); err != nil {
		t.Fatalf("DisconnectMember on a closed connection: %v", err)
	}

	fetched, err := h.GetRoom(context.Background(), room.ID)
	if err != nil || fetched == nil {
		t.Fatalf("room should still exist while user-2 remains: %v", err)
	}
	if fetched.IsMember("host-1") && string(fetched.HostID) == "host-1" {
		t.Error("the departed host is still the host")
	}
	if string(fetched.HostID) != "user-2" {
		t.Errorf("host = %q, want user-2 elected", fetched.HostID)
	}
}

func TestHub_DisconnectMember_Success(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	// Two connected members so removing one leaves the room alive.
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", fakes.NewFakeMessageSender())
	_, _ = h.ConnectMember(ctx, room.ID, "user-3", "Bob", fakes.NewFakeMessageSender())

	if err := h.DisconnectMember(ctx, room.ID, "user-2"); err != nil {
		t.Fatalf("DisconnectMember: %v", err)
	}

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil || fetched == nil {
		t.Fatalf("room should still exist while user-3 remains: %v", err)
	}
	for _, m := range fetched.Members {
		if m.UserID == "user-2" {
			t.Error("user-2 should be removed after DisconnectMember")
		}
	}
}

func TestHub_DisconnectMember_DeletesRoomWhenLastLeaves(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", fakes.NewFakeMessageSender())

	// user-2 is the only connected member; disconnecting must delete the room
	// rather than leak its row (the WS-close path, not the LeaveRoom button).
	if err := h.DisconnectMember(ctx, room.ID, "user-2"); err != nil {
		t.Fatalf("DisconnectMember: %v", err)
	}

	if fetched, err := h.GetRoom(ctx, room.ID); err == nil && fetched != nil {
		t.Errorf("room should be deleted after its last member disconnected, got %d members", len(fetched.Members))
	}
}

func TestHub_DisconnectMember_NonexistentRoom(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	err := h.DisconnectMember(ctx, "bad-room", "user-1")
	if err == nil {
		t.Error("expected error disconnecting from nonexistent room")
	}
}

func TestHub_DisconnectMember_NonexistentUser_NoError(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")

	err := h.DisconnectMember(ctx, room.ID, "nobody")
	if err != nil {
		t.Errorf("DisconnectMember for absent user should not error, got: %v", err)
	}
}

// --- JoinRoom discrete tests ---

func TestHub_JoinRoom_Idempotent(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_ = h.JoinRoom(ctx, room.ID, "user-2", "Alice")
	_ = h.JoinRoom(ctx, room.ID, "user-2", "Alice")

	fetched, _ := h.GetRoom(ctx, room.ID)
	count := 0
	for _, m := range fetched.Members {
		if m.UserID == "user-2" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 entry for user-2 after double join, got %d", count)
	}
}

func TestHub_JoinRoom_SetsJoinedAt(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	before := time.Now().UTC()
	_ = h.JoinRoom(ctx, room.ID, "user-2", "Alice")

	fetched, _ := h.GetRoom(ctx, room.ID)
	for _, m := range fetched.Members {
		if m.UserID == "user-2" {
			if m.JoinedAt.Before(before) {
				t.Errorf("JoinedAt = %v, expected >= %v", m.JoinedAt, before)
			}
			return
		}
	}
	t.Error("user-2 not found in members")
}

func TestHub_HandleCommand_BroadcastsToOthers(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	senderSender := fakes.NewFakeMessageSender()
	otherSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", senderSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", otherSender)

	// Clear connect messages
	senderSender.Messages = nil
	otherSender.Messages = nil

	_, err := h.HandleCommand(ctx, room.ID, "host-1", "Host", syncplay.SyncCommand{
		Type: "play", PositionMs: 1000,
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	// Sender should NOT receive broadcast
	if len(senderSender.Received()) != 0 {
		t.Errorf("sender got %d messages, want 0", len(senderSender.Received()))
	}
	// Other member should receive broadcast
	msgs := otherSender.Received()
	if len(msgs) != 1 {
		t.Fatalf("other got %d messages, want 1", len(msgs))
	}
	var cmd syncplay.SyncCommand
	if err := json.Unmarshal(msgs[0], &cmd); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cmd.Type != "play" {
		t.Errorf("cmd.Type = %q, want %q", cmd.Type, "play")
	}
}

func TestHub_ConnectMember_BroadcastsJoin(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	hostSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", hostSender)
	hostSender.Messages = nil

	newSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", newSender)

	// Host should get member_joined
	msgs := hostSender.Received()
	if len(msgs) != 1 {
		t.Fatalf("host got %d messages, want 1 (member_joined)", len(msgs))
	}
	var cmd syncplay.SyncCommand
	_ = json.Unmarshal(msgs[0], &cmd)
	if cmd.Type != "member_joined" {
		t.Errorf("type = %q, want member_joined", cmd.Type)
	}
	if cmd.UserID != "user-2" {
		t.Errorf("userID = %q, want user-2", cmd.UserID)
	}
}

func TestHub_ConnectMember_LogsWarnOnSenderError(t *testing.T) {
	buf := captureSlog(t)
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	failSender := &fakes.FakeMessageSender{Err: errors.New("send boom")}

	_, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", failSender)
	if err != nil {
		t.Fatalf("ConnectMember: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("send initial state failed")) {
		t.Errorf("expected warn log for failed send, got: %q", buf.String())
	}
}

func TestHub_ConnectMember_NoWarnOnSuccessfulSend(t *testing.T) {
	buf := captureSlog(t)
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	okSender := fakes.NewFakeMessageSender()

	_, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", okSender)
	if err != nil {
		t.Fatalf("ConnectMember: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("send initial state failed")) {
		t.Errorf("unexpected warn log on successful send: %q", buf.String())
	}
}

func TestHub_Broadcast_LogsWarnOnSenderError(t *testing.T) {
	buf := captureSlog(t)
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	hostSender := fakes.NewFakeMessageSender()
	failSender := &fakes.FakeMessageSender{Err: errors.New("broadcast boom")}
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", hostSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", failSender)
	buf.Reset()

	_, err := h.HandleCommand(ctx, room.ID, "host-1", "Host", syncplay.SyncCommand{
		Type: "play", PositionMs: 1000,
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("syncplay: send failed")) {
		t.Errorf("expected broadcast warn log, got: %q", buf.String())
	}
}

func TestHub_Broadcast_NoWarnOnSuccessfulSend(t *testing.T) {
	buf := captureSlog(t)
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	hostSender := fakes.NewFakeMessageSender()
	otherSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", hostSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", otherSender)
	buf.Reset()

	_, err := h.HandleCommand(ctx, room.ID, "host-1", "Host", syncplay.SyncCommand{
		Type: "play", PositionMs: 1000,
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("syncplay: send failed")) {
		t.Errorf("unexpected broadcast warn log: %q", buf.String())
	}
}

func TestHub_DisconnectMember_RemovesFromConns(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	hostSender := fakes.NewFakeMessageSender()
	otherSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", hostSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", otherSender)

	// Disconnect user-2: must remove from h.conns so subsequent broadcasts skip them.
	_ = h.DisconnectMember(ctx, room.ID, "user-2")
	otherSender.Messages = nil
	hostSender.Messages = nil

	// Host sends play - user-2 must NOT receive because they are no longer in conns.
	_, err := h.HandleCommand(ctx, room.ID, "host-1", "Host", syncplay.SyncCommand{
		Type: "play", PositionMs: 2000,
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if got := len(otherSender.Received()); got != 0 {
		t.Errorf("disconnected user got %d messages, want 0", got)
	}
}

func TestHub_DisconnectMember_NoConns_NoPanic(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	// No ConnectMember called → h.conns[roomID] is nil.
	if err := h.DisconnectMember(ctx, room.ID, "ghost"); err != nil {
		t.Errorf("DisconnectMember on room with no conns: %v", err)
	}
}

func TestHub_ConnectMember_SenderErrorSwallowed(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	failSender := &fakes.FakeMessageSender{Err: errors.New("send boom")}

	got, err := h.ConnectMember(ctx, room.ID, "user-2", "Alice", failSender)
	if err != nil {
		t.Fatalf("ConnectMember should swallow sender error, got %v", err)
	}
	if got.ID != room.ID {
		t.Errorf("room ID = %q, want %q", got.ID, room.ID)
	}
	// Member must still be persisted despite failed state send.
	found := false
	for _, m := range got.Members {
		if m.UserID == "user-2" {
			found = true
		}
	}
	if !found {
		t.Error("user-2 must be added even when initial state send fails")
	}
}

func TestHub_Broadcast_ContinuesWhenOneSenderFails(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	hostSender := fakes.NewFakeMessageSender()
	failSender := &fakes.FakeMessageSender{Err: errors.New("boom")}
	goodSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", hostSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", failSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-3", "Bob", goodSender)
	goodSender.Messages = nil

	_, err := h.HandleCommand(ctx, room.ID, "host-1", "Host", syncplay.SyncCommand{
		Type: "play", PositionMs: 1000,
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if got := len(goodSender.Received()); got != 1 {
		t.Errorf("good sender got %d messages, want 1 (broadcast must continue past failing sender)", got)
	}
}

func TestHub_DisconnectMember_BroadcastsLeave(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	hostSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", hostSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", fakes.NewFakeMessageSender())
	hostSender.Messages = nil

	_ = h.DisconnectMember(ctx, room.ID, "user-2")

	msgs := hostSender.Received()
	if len(msgs) != 1 {
		t.Fatalf("host got %d messages, want 1 (member_left)", len(msgs))
	}
	var cmd syncplay.SyncCommand
	_ = json.Unmarshal(msgs[0], &cmd)
	if cmd.Type != "member_left" {
		t.Errorf("type = %q, want member_left", cmd.Type)
	}
}

// --- Queue / playlist room tests ---

func TestHub_CreateRoomFromPlaylist_OK(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	tracks := []string{"t1", "t2", "t3"}

	room, err := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "My Playlist", tracks)
	if err != nil {
		t.Fatalf("CreateRoomFromPlaylist: %v", err)
	}
	if room.MediaType != "music" {
		t.Errorf("MediaType = %q, want music", room.MediaType)
	}
	if string(room.MediaID) != "t1" {
		t.Errorf("MediaID = %q, want t1", room.MediaID)
	}
	if room.PlaylistID == nil || *room.PlaylistID != "pl-1" {
		t.Errorf("PlaylistID = %v, want pl-1", room.PlaylistID)
	}
	if len(room.Queue) != 3 {
		t.Errorf("Queue len = %d, want 3", len(room.Queue))
	}
	if room.TrackIndex != 0 {
		t.Errorf("TrackIndex = %d, want 0", room.TrackIndex)
	}
}

func TestHub_CreateRoomFromPlaylist_Empty(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	_, err := h.CreateRoomFromPlaylist(context.Background(), "host-1", "pl-1", "X", nil)
	if err == nil {
		t.Error("expected error for empty playlist")
	}
}

func TestHub_HandleNext_AdvancesQueue(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	room, err := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "X", []string{"t1", "t2", "t3"})
	if err != nil {
		t.Fatalf("CreateRoomFromPlaylist: %v", err)
	}
	cmd, err := h.HandleCommand(ctx, room.ID, "host-1", "host", syncplay.SyncCommand{Type: "next"})
	if err != nil {
		t.Fatalf("HandleCommand next: %v", err)
	}
	if cmd.TrackIndex != 1 {
		t.Errorf("TrackIndex = %d, want 1", cmd.TrackIndex)
	}
	if cmd.MediaID != "t2" {
		t.Errorf("MediaID = %q, want t2", cmd.MediaID)
	}
	got, _ := h.GetRoom(ctx, room.ID)
	if got.TrackIndex != 1 || string(got.MediaID) != "t2" {
		t.Errorf("room state mismatch: idx=%d media=%s", got.TrackIndex, got.MediaID)
	}
}

func TestHub_HandleNext_EchoesToHostSender(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	room, err := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "X", []string{"t1", "t2", "t3"})
	if err != nil {
		t.Fatalf("CreateRoomFromPlaylist: %v", err)
	}
	hostSender := fakes.NewFakeMessageSender()
	otherSender := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", hostSender)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", otherSender)
	hostSender.Messages = nil
	otherSender.Messages = nil

	if _, err := h.HandleCommand(ctx, room.ID, "host-1", "Host", syncplay.SyncCommand{Type: "next"}); err != nil {
		t.Fatalf("HandleCommand next: %v", err)
	}

	// The host that sent "next" MUST receive the echo - the frontend advances the
	// queue only on this broadcast, not locally, so skipping the sender (as
	// play/pause/seek do) left the host stuck on the same track ("can't skip").
	assertGotNext := func(name string, s *fakes.FakeMessageSender) {
		t.Helper()
		msgs := s.Received()
		if len(msgs) != 1 {
			t.Fatalf("%s got %d messages, want 1 (the next echo)", name, len(msgs))
		}
		var cmd syncplay.SyncCommand
		if err := json.Unmarshal(msgs[0], &cmd); err != nil {
			t.Fatalf("%s unmarshal: %v", name, err)
		}
		if cmd.Type != "next" || cmd.TrackIndex != 1 || cmd.MediaID != "t2" {
			t.Errorf("%s got %+v, want next idx=1 media=t2", name, cmd)
		}
	}
	assertGotNext("host/sender", hostSender)
	assertGotNext("other member", otherSender)
}

func TestHub_HandleNext_AtEndIsNoop(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	room, _ := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "X", []string{"t1"})
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "host", syncplay.SyncCommand{Type: "next"})
	got, _ := h.GetRoom(ctx, room.ID)
	if got.TrackIndex != 0 {
		t.Errorf("TrackIndex = %d, want 0 (no advance past end)", got.TrackIndex)
	}
}

func TestHub_HandlePrev_GoesBack(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	room, _ := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "X", []string{"t1", "t2"})
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "host", syncplay.SyncCommand{Type: "next"})
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "host", syncplay.SyncCommand{Type: "prev"})
	got, _ := h.GetRoom(ctx, room.ID)
	if got.TrackIndex != 0 {
		t.Errorf("TrackIndex = %d, want 0", got.TrackIndex)
	}
}

func TestHub_HandleSetIndex_OK(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	room, _ := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "X", []string{"t1", "t2", "t3"})
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "host", syncplay.SyncCommand{Type: "setIndex", TrackIndex: 2})
	got, _ := h.GetRoom(ctx, room.ID)
	if got.TrackIndex != 2 || string(got.MediaID) != "t3" {
		t.Errorf("idx=%d media=%s, want 2/t3", got.TrackIndex, got.MediaID)
	}
}

func TestHub_HandleSetIndex_OutOfRange(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	room, _ := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "X", []string{"t1", "t2"})
	_, _ = h.HandleCommand(ctx, room.ID, "host-1", "host", syncplay.SyncCommand{Type: "setIndex", TrackIndex: 99})
	got, _ := h.GetRoom(ctx, room.ID)
	if got.TrackIndex != 0 {
		t.Errorf("TrackIndex = %d, want 0 (oor noop)", got.TrackIndex)
	}
}

func TestHub_HandleNext_NonHostIgnored(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()
	room, _ := h.CreateRoomFromPlaylist(ctx, "host-1", "pl-1", "X", []string{"t1", "t2"})
	_, _ = h.HandleCommand(ctx, room.ID, "guest-1", "guest", syncplay.SyncCommand{Type: "next"})
	got, _ := h.GetRoom(ctx, room.ID)
	if got.TrackIndex != 0 {
		t.Errorf("TrackIndex = %d, want 0 (guest cannot advance)", got.TrackIndex)
	}
}

// --- Bug #2: host departure → host reassignment ---

// (a) Host disconnects with ≥1 member remaining → HostID reassigned to oldest member.
// "oldest" = earliest JoinedAt; tie-break = lexicographic UserID ascending.
func TestHub_DisconnectMember_HostDeparture_ReassignsToOldest(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	s1 := fakes.NewFakeMessageSender()
	s2 := fakes.NewFakeMessageSender()
	s3 := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", s1)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", s2) // joins after host
	_, _ = h.ConnectMember(ctx, room.ID, "user-3", "Bob", s3)   // joins last

	if err := h.DisconnectMember(ctx, room.ID, "host-1"); err != nil {
		t.Fatalf("DisconnectMember: %v", err)
	}

	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	// user-2 joined before user-3; if timestamps are equal "user-2" < "user-3"
	// by lexicographic order, so user-2 wins in both cases.
	if string(fetched.HostID) != "user-2" {
		t.Errorf("HostID = %q after host departure, want user-2 (oldest remaining)", fetched.HostID)
	}
}

// (b) After host departure the new host's play/pause commands are accepted.
func TestHub_DisconnectMember_NewHost_CanSendCommands(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	s1 := fakes.NewFakeMessageSender()
	s2 := fakes.NewFakeMessageSender()
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", s1)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", s2)

	// Host disconnects → user-2 is elected new host.
	if err := h.DisconnectMember(ctx, room.ID, "host-1"); err != nil {
		t.Fatalf("DisconnectMember: %v", err)
	}

	// user-2 (now host) sends play - must be accepted.
	if _, err := h.HandleCommand(ctx, room.ID, "user-2", "Alice", syncplay.SyncCommand{
		Type: "play", PositionMs: 5000,
	}); err != nil {
		t.Fatalf("HandleCommand by new host: %v", err)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	if !fetched.State.Playing {
		t.Error("new host's play command must be accepted (room was frozen before fix)")
	}
	if fetched.State.PositionMs != 5000 {
		t.Errorf("PositionMs = %d, want 5000", fetched.State.PositionMs)
	}
}

// (c) Host is the sole member → room is cleaned up as before (no regression).
func TestHub_DisconnectMember_HostAlone_DeletesRoom(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", fakes.NewFakeMessageSender())

	if err := h.DisconnectMember(ctx, room.ID, "host-1"); err != nil {
		t.Fatalf("DisconnectMember: %v", err)
	}

	if _, err := h.GetRoom(ctx, room.ID); err == nil {
		t.Error("room must be deleted when the last member (host) disconnects")
	}
}

// --- Bug #3: double-tab does not orphan the member ---

// (d) Two connections for same user; closing one keeps the member present and
// the second connection still receives broadcasts.
func TestHub_DisconnectMember_DoubleTab_MemberStays(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	tab1 := fakes.NewFakeMessageSender()
	tab2 := fakes.NewFakeMessageSender()
	otherSender := fakes.NewFakeMessageSender()

	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", tab1)
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", otherSender)
	// host-1 opens a second tab: overwrites tab1's sender in conns, count → 2.
	_, _ = h.ConnectMember(ctx, room.ID, "host-1", "Host", tab2)
	otherSender.Messages = nil

	// tab1 closes.
	if err := h.DisconnectMember(ctx, room.ID, "host-1"); err != nil {
		t.Fatalf("DisconnectMember tab1: %v", err)
	}

	// host-1 must remain in the room (tab2 is still open).
	fetched, err := h.GetRoom(ctx, room.ID)
	if err != nil {
		t.Fatalf("GetRoom after tab1 disconnect: %v", err)
	}
	found := false
	for _, m := range fetched.Members {
		if m.UserID == "host-1" {
			found = true
		}
	}
	if !found {
		t.Error("host-1 must remain in room while tab2 is still open")
	}

	// No member_left must have been broadcast (host-1 is still connected).
	if got := len(otherSender.Received()); got != 0 {
		t.Errorf("member_left broadcast to other members while user still has tab2 open: got %d messages", got)
	}

	// tab2 is still the active connection: broadcasts must reach user-2.
	otherSender.Messages = nil
	if _, err := h.HandleCommand(ctx, room.ID, "host-1", "Host", syncplay.SyncCommand{
		Type: "play", PositionMs: 1000,
	}); err != nil {
		t.Fatalf("HandleCommand via tab2: %v", err)
	}
	if got := len(otherSender.Received()); got != 1 {
		t.Errorf("broadcast via remaining connection: user-2 got %d messages, want 1", got)
	}
}

// The Hub is shared state driven by one goroutine per WebSocket connection, but
// unlike DeviceRegistry it had no concurrency test at all - so the repo's -race
// runs never actually exercised concurrent Hub access. This drives connect /
// command / broadcast / disconnect from many goroutines at once against the same
// room, which is exactly the real shape: N members of one room acting together.
func TestHub_ConcurrentMembersOnOneRoom(t *testing.T) {
	repo := newFakeRoomRepo()
	h := NewHub(repo)
	ctx := context.Background()

	room, err := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Anchor connection, held for the whole test: DisconnectMember deletes the
	// room once the LAST member leaves, and the workers below connect and
	// disconnect in a tight loop. Without an anchor the first worker to find
	// itself alone destroys the room, every later ConnectMember fails with
	// "room not found", and the test measures nothing.
	//
	// That was not hypothetical - it is what happened. The test only passed
	// under `-race`, whose scheduling overhead kept the workers overlapping by
	// luck; a plain `go test` tripped the vacuous-pass guard below. A
	// concurrency test whose coverage depends on the race detector's timing
	// isn't testing concurrency, so the room lifetime is now explicit.
	anchor := fakes.NewFakeMessageSender()
	if _, aErr := h.ConnectMember(ctx, room.ID, "anchor", "Anchor", anchor); aErr != nil {
		t.Fatalf("anchor ConnectMember: %v", aErr)
	}
	defer func() { _ = h.DisconnectMember(ctx, room.ID, "anchor") }()

	const workers = 12
	const iters = 60
	// Guard against a vacuous pass: if every ConnectMember errored, the loop
	// below would skip all the interesting work and the test would still be
	// green while exercising nothing.
	var connected atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := range workers {
		go func(w int) {
			defer wg.Done()
			// Worker 0 acts as the host so the host-only command paths are
			// exercised concurrently with everyone else's rejected attempts.
			userID := "host-1"
			if w != 0 {
				userID = "member-" + string(rune('A'+w))
			}
			for range iters {
				sender := fakes.NewFakeMessageSender()
				if _, cErr := h.ConnectMember(ctx, room.ID, userID, "U", sender); cErr != nil {
					continue
				}
				connected.Add(1)
				_, _ = h.HandleCommand(ctx, room.ID, userID, "U", syncplay.SyncCommand{Type: "play"})
				_, _ = h.HandleCommand(ctx, room.ID, userID, "U", syncplay.SyncCommand{Type: "seek", PositionMs: 12_000})
				_, _ = h.GetRoom(ctx, room.ID)
				_ = h.DisconnectMember(ctx, room.ID, userID)
			}
		}(w)
	}
	wg.Wait()

	if got := connected.Load(); got < workers {
		t.Fatalf("only %d successful connects across %d workers - the test never exercised concurrent hub access", got, workers)
	}
}
