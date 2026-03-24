package syncplay

import (
	"context"
	"time"

	"github.com/gravity-zero/evey/internal/domain/syncplay"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

type Hub struct {
	repo syncplay.RoomRepository
}

func NewHub(repo syncplay.RoomRepository) *Hub {
	return &Hub{repo: repo}
}

func (h *Hub) CreateRoom(ctx context.Context, hostID, mediaID, name string) (*syncplay.Room, error) {
	room := &syncplay.Room{
		ID:        id.New("room-"),
		Code:      syncplay.GenerateCode(),
		Name:      name,
		HostID:    hostID,
		MediaID:   mediaID,
		CreatedAt: time.Now().UTC(),
		State:     syncplay.PlaybackState{UpdatedAt: time.Now().UTC()},
	}
	if err := h.repo.Create(ctx, room); err != nil {
		return nil, err
	}
	return room, nil
}

func (h *Hub) GetRoom(ctx context.Context, roomID string) (*syncplay.Room, error) {
	return h.repo.Get(ctx, roomID)
}

func (h *Hub) GetRoomByCode(ctx context.Context, code string) (*syncplay.Room, error) {
	return h.repo.GetByCode(ctx, code)
}

func (h *Hub) JoinRoom(ctx context.Context, roomID, userID, username string) error {
	member := syncplay.Member{
		UserID:   userID,
		Username: username,
		JoinedAt: time.Now().UTC(),
	}
	return h.repo.AddMember(ctx, roomID, member)
}

func (h *Hub) LeaveRoom(ctx context.Context, roomID, userID string) error {
	if err := h.repo.RemoveMember(ctx, roomID, userID); err != nil {
		return err
	}
	room, err := h.repo.Get(ctx, roomID)
	if err != nil {
		return err
	}
	if len(room.Members) == 0 {
		return h.repo.Delete(ctx, roomID)
	}
	return nil
}

// HandleCommand processes a playback command (play/pause/seek) from a user.
// Returns the updated command (with masked username) to broadcast to other members.
func (h *Hub) HandleCommand(ctx context.Context, roomID, userID, username string, cmd syncplay.SyncCommand) (syncplay.SyncCommand, error) {
	cmd.UserID = userID
	cmd.Username = syncplay.MaskUsername(username)
	cmd.RoomID = roomID

	switch cmd.Type {
	case "play", "pause", "seek":
		state := syncplay.PlaybackState{
			Playing:    cmd.Type == "play",
			PositionMs: cmd.PositionMs,
			UpdatedAt:  time.Now().UTC(),
			UpdatedBy:  userID,
		}
		if cmd.Type == "pause" {
			state.Playing = false
		}
		if cmd.Type == "seek" {
			if existing, err := h.repo.Get(ctx, roomID); err == nil {
				state.Playing = existing.State.Playing
			}
		}
		if err := h.repo.UpdateState(ctx, roomID, state); err != nil {
			return cmd, err
		}
	}
	return cmd, nil
}

// ConnectMember adds a member to a room and returns the current state.
func (h *Hub) ConnectMember(ctx context.Context, roomID, userID, username string) (*syncplay.Room, error) {
	if err := h.repo.AddMember(ctx, roomID, syncplay.Member{
		UserID:   userID,
		Username: username,
		JoinedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	return h.repo.Get(ctx, roomID)
}

// DisconnectMember removes a member from a room.
func (h *Hub) DisconnectMember(ctx context.Context, roomID, userID string) error {
	return h.repo.RemoveMember(ctx, roomID, userID)
}
