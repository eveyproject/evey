package syncplay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/syncplay"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type connEntry struct {
	sender   syncplay.MessageSender
	username string
}

type Hub struct {
	repo    syncplay.RoomRepository
	mu      sync.RWMutex
	conns   map[string]map[string]*connEntry // roomID → userID → entry
}

func NewHub(repo syncplay.RoomRepository) *Hub {
	return &Hub{
		repo:  repo,
		conns: make(map[string]map[string]*connEntry),
	}
}

func (h *Hub) CreateRoom(ctx context.Context, hostID, mediaID, name string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.CreateRoom")
	defer span.End()
	return h.CreateRoomTyped(ctx, hostID, mediaID, name, "video")
}

func (h *Hub) CreateRoomTyped(ctx context.Context, hostID, mediaID, name, mediaType string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.CreateRoomTyped")
	defer span.End()
	code, err := syncplay.GenerateCode()
	if err != nil {
		return nil, fmt.Errorf("generate room code: %w", err)
	}
	if mediaType == "" {
		mediaType = "video"
	}
	room := &syncplay.Room{
		ID:        id.New("room-"),
		Code:      code,
		Name:      name,
		HostID:    shared.UserID(hostID),
		MediaID:   shared.MediaItemID(mediaID),
		MediaType: mediaType,
		CreatedAt: time.Now().UTC(),
		State:     syncplay.PlaybackState{UpdatedAt: time.Now().UTC()},
	}
	if err := h.repo.Create(ctx, room); err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	return room, nil
}

// CreateRoomFromPlaylist creates a music room with a queue of track IDs from a playlist.
// The first track becomes the current MediaID; track_index starts at 0.
// trackIDs must be non-empty.
func (h *Hub) CreateRoomFromPlaylist(ctx context.Context, hostID, playlistID, name string, trackIDs []string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.CreateRoomFromPlaylist")
	defer span.End()
	if len(trackIDs) == 0 {
		return nil, fmt.Errorf("playlist has no tracks")
	}
	code, err := syncplay.GenerateCode()
	if err != nil {
		return nil, fmt.Errorf("generate room code: %w", err)
	}
	queue := make([]shared.MediaItemID, 0, len(trackIDs))
	for _, t := range trackIDs {
		queue = append(queue, shared.MediaItemID(t))
	}
	plID := playlistID
	room := &syncplay.Room{
		ID:         id.New("room-"),
		Code:       code,
		Name:       name,
		HostID:     shared.UserID(hostID),
		MediaID:    shared.MediaItemID(trackIDs[0]),
		MediaType:  "music",
		PlaylistID: &plID,
		Queue:      queue,
		TrackIndex: 0,
		CreatedAt:  time.Now().UTC(),
		State:      syncplay.PlaybackState{UpdatedAt: time.Now().UTC()},
	}
	if err := h.repo.Create(ctx, room); err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	return room, nil
}

func (h *Hub) GetRoom(ctx context.Context, roomID string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.GetRoom")
	defer span.End()
	return h.repo.Get(ctx, roomID)
}

func (h *Hub) GetRoomByCode(ctx context.Context, code string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.GetRoomByCode")
	defer span.End()
	return h.repo.GetByCode(ctx, code)
}

func (h *Hub) JoinRoom(ctx context.Context, roomID, userID, username string) error {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.JoinRoom")
	defer span.End()
	member := syncplay.Member{
		UserID:   shared.UserID(userID),
		Username: username,
		JoinedAt: time.Now().UTC(),
	}
	if err := h.repo.AddMember(ctx, roomID, member); err != nil {
		return fmt.Errorf("join room: %w", err)
	}
	return nil
}

func (h *Hub) LeaveRoom(ctx context.Context, roomID, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.LeaveRoom")
	defer span.End()
	if err := h.repo.RemoveMember(ctx, roomID, userID); err != nil {
		return fmt.Errorf("leave room remove member: %w", err)
	}
	room, err := h.repo.Get(ctx, roomID)
	if err != nil {
		return fmt.Errorf("leave room get room: %w", err)
	}
	if len(room.Members) == 0 {
		return h.repo.Delete(ctx, roomID)
	}
	return nil
}

// ConnectMember registers a WebSocket sender, adds the member to the room,
// sends the current state to the new member, and broadcasts member_joined.
func (h *Hub) ConnectMember(ctx context.Context, roomID, userID, username string, sender syncplay.MessageSender) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.ConnectMember")
	defer span.End()
	if err := h.repo.AddMember(ctx, roomID, syncplay.Member{
		UserID:   shared.UserID(userID),
		Username: username,
		JoinedAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}

	h.mu.Lock()
	if h.conns[roomID] == nil {
		h.conns[roomID] = make(map[string]*connEntry)
	}
	h.conns[roomID][userID] = &connEntry{sender: sender, username: username}
	h.mu.Unlock()

	room, err := h.repo.Get(ctx, roomID)
	if err != nil {
		return nil, err
	}

	stateMsg := syncplay.SyncCommand{
		Type:       "state",
		Playing:    room.State.Playing,
		PositionMs: room.State.PositionMs,
	}
	if data, mErr := json.Marshal(stateMsg); mErr == nil {
		if err := sender.SendMessage(data); err != nil {
			slog.Warn("send initial state failed", slog.String("roomID", roomID), slog.String("userID", userID), slog.Any("err", err))
		}
	}

	joinMsg := syncplay.SyncCommand{
		Type:     "member_joined",
		UserID:   userID,
		Username: syncplay.MaskUsername(username),
		RoomID:   roomID,
	}
	h.broadcast(roomID, userID, joinMsg)

	return room, nil
}

// DisconnectMember removes the sender and broadcasts member_left.
func (h *Hub) DisconnectMember(ctx context.Context, roomID, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.DisconnectMember")
	defer span.End()
	h.mu.Lock()
	if m := h.conns[roomID]; m != nil {
		delete(m, userID)
		if len(m) == 0 {
			delete(h.conns, roomID)
		}
	}
	h.mu.Unlock()

	leaveMsg := syncplay.SyncCommand{
		Type:   "member_left",
		UserID: userID,
		RoomID: roomID,
	}
	h.broadcast(roomID, userID, leaveMsg)

	return h.repo.RemoveMember(ctx, roomID, userID)
}

// HandleCommand processes a playback command and broadcasts to other room members.
func (h *Hub) HandleCommand(ctx context.Context, roomID, userID, username string, cmd syncplay.SyncCommand) (syncplay.SyncCommand, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.HandleCommand")
	defer span.End()
	cmd.UserID = userID
	cmd.Username = syncplay.MaskUsername(username)
	cmd.RoomID = roomID

	switch cmd.Type {
	case "play", "pause", "seek":
		state := syncplay.PlaybackState{
			Playing:    cmd.Type == "play",
			PositionMs: cmd.PositionMs,
			UpdatedAt:  time.Now().UTC(),
			UpdatedBy:  shared.UserID(userID),
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
			return cmd, fmt.Errorf("update room state: %w", err)
		}
		h.broadcast(roomID, userID, cmd)
	case "next", "prev", "setIndex":
		room, err := h.repo.Get(ctx, roomID)
		if err != nil || room == nil {
			return cmd, fmt.Errorf("get room: %w", err)
		}
		if len(room.Queue) == 0 {
			return cmd, nil
		}
		// host-only check
		if string(room.HostID) != userID {
			return cmd, nil
		}
		idx := room.TrackIndex
		switch cmd.Type {
		case "next":
			idx++
		case "prev":
			idx--
		case "setIndex":
			idx = cmd.TrackIndex
		}
		if idx < 0 || idx >= len(room.Queue) {
			return cmd, nil
		}
		mediaID := string(room.Queue[idx])
		if err := h.repo.UpdateTrackIndex(ctx, roomID, idx, mediaID); err != nil {
			return cmd, fmt.Errorf("update track index: %w", err)
		}
		// reset playback state for new track
		state := syncplay.PlaybackState{
			Playing:    true,
			PositionMs: 0,
			UpdatedAt:  time.Now().UTC(),
			UpdatedBy:  shared.UserID(userID),
		}
		if err := h.repo.UpdateState(ctx, roomID, state); err != nil {
			return cmd, fmt.Errorf("update state: %w", err)
		}
		cmd.TrackIndex = idx
		cmd.MediaID = mediaID
		h.broadcast(roomID, userID, cmd)
	}
	return cmd, nil
}

func (h *Hub) broadcast(roomID, senderUserID string, cmd syncplay.SyncCommand) {
	data, err := json.Marshal(cmd)
	if err != nil {
		return
	}

	h.mu.RLock()
	room := h.conns[roomID]
	entries := make(map[string]*connEntry, len(room))
	for uid, e := range room {
		entries[uid] = e
	}
	h.mu.RUnlock()

	for uid, e := range entries {
		if uid == senderUserID {
			continue
		}
		if err := e.sender.SendMessage(data); err != nil {
			slog.Warn("syncplay: send failed", slog.String("room", roomID), slog.String("user", uid), slog.Any("err", err))
		}
	}
}
