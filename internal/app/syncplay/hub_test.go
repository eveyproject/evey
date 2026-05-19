package syncplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
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

func (r *fakeRoomRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, id)
	return nil
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
	if len(room.Code) != 5 {
		t.Errorf("Code length = %d, want 5", len(room.Code))
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

// --- DisconnectMember tests ---

func TestHub_DisconnectMember_Success(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	_, _ = h.ConnectMember(ctx, room.ID, "user-2", "Alice", fakes.NewFakeMessageSender())

	if err := h.DisconnectMember(ctx, room.ID, "user-2"); err != nil {
		t.Fatalf("DisconnectMember: %v", err)
	}

	fetched, _ := h.GetRoom(ctx, room.ID)
	for _, m := range fetched.Members {
		if m.UserID == "user-2" {
			t.Error("user-2 should be removed after DisconnectMember")
		}
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

	// Host sends play — user-2 must NOT receive because they are no longer in conns.
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
