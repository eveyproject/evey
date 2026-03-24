package syncplay

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gravity-zero/evey/internal/domain/syncplay"
)

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
		if m.UserID != userID {
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

	_, err := h.GetRoomByCode(ctx, "ZZZZZZ")
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
