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
	repo       syncplay.RoomRepository
	mu         sync.RWMutex
	conns      map[string]map[string]*connEntry // roomID → userID → entry (latest sender)
	connCounts map[string]map[string]int        // roomID → userID → active connection count
}

func NewHub(repo syncplay.RoomRepository) *Hub {
	return &Hub{
		repo:       repo,
		conns:      make(map[string]map[string]*connEntry),
		connCounts: make(map[string]map[string]int),
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
	room, err := syncplay.NewRoom(id.New("room-"), code, name, shared.UserID(hostID), shared.MediaItemID(mediaID), mediaType)
	if err != nil {
		return nil, fmt.Errorf("build room: %w", err)
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
	room, err := syncplay.NewRoom(id.New("room-"), code, name, shared.UserID(hostID), shared.MediaItemID(trackIDs[0]), "music")
	if err != nil {
		return nil, fmt.Errorf("build room: %w", err)
	}
	plID := playlistID
	room.PlaylistID = &plID
	room.Queue = queue
	if err := h.repo.Create(ctx, room); err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	return room, nil
}

// CreateRoomFromQueue creates a music room from an explicit ordered list of
// track IDs (a mix of albums, individual tracks and playlists assembled by the
// client). The first track becomes the current MediaID; track_index starts at
// 0. trackIDs must be non-empty. Unlike CreateRoomFromPlaylist the room is not
// bound to a source playlist.
func (h *Hub) CreateRoomFromQueue(ctx context.Context, hostID, name string, trackIDs []string) (*syncplay.Room, error) {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.CreateRoomFromQueue")
	defer span.End()
	if len(trackIDs) == 0 {
		return nil, fmt.Errorf("queue has no tracks")
	}
	code, err := syncplay.GenerateCode()
	if err != nil {
		return nil, fmt.Errorf("generate room code: %w", err)
	}
	queue := make([]shared.MediaItemID, 0, len(trackIDs))
	for _, t := range trackIDs {
		queue = append(queue, shared.MediaItemID(t))
	}
	room, err := syncplay.NewRoom(id.New("room-"), code, name, shared.UserID(hostID), shared.MediaItemID(trackIDs[0]), "music")
	if err != nil {
		return nil, fmt.Errorf("build room: %w", err)
	}
	room.Queue = queue
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
	if _, _, err := h.repo.RemoveMemberAndDeleteIfEmpty(ctx, roomID, userID); err != nil {
		return fmt.Errorf("leave room: %w", err)
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
	if h.connCounts[roomID] == nil {
		h.connCounts[roomID] = make(map[string]int)
	}
	h.conns[roomID][userID] = &connEntry{sender: sender, username: username}
	h.connCounts[roomID][userID]++
	h.mu.Unlock()

	room, err := h.repo.Get(ctx, roomID)
	if err != nil {
		// Get failed *after* we registered the connection and added the room
		// member. Roll both back: the ws handler returns before arming its
		// defer DisconnectMember, so without this the dead connection lingers
		// in h.conns forever (every future broadcast retries a send to it) and
		// the room keeps a phantom member.
		h.mu.Lock()
		if m := h.conns[roomID]; m != nil {
			delete(m, userID)
			if len(m) == 0 {
				delete(h.conns, roomID)
			}
		}
		if c := h.connCounts[roomID]; c != nil {
			c[userID]--
			if c[userID] <= 0 {
				delete(c, userID)
			}
			if len(c) == 0 {
				delete(h.connCounts, roomID)
			}
		}
		h.mu.Unlock()
		if rmErr := h.repo.RemoveMember(ctx, roomID, userID); rmErr != nil {
			slog.Warn("syncplay: rollback RemoveMember after Get failure",
				slog.String("roomID", roomID), slog.String("userID", userID), slog.Any("err", rmErr))
		}
		return nil, err
	}

	// The stored position is a snapshot taken at State.UpdatedAt (when the last
	// play/pause/seek landed). While the room is playing it keeps advancing, so a
	// mid-session joiner must seek to the LIVE position (snapshot + time elapsed
	// since), not the stale snapshot, or it starts behind everyone else.
	posMs := room.State.PositionMs
	if room.State.Playing && !room.State.UpdatedAt.IsZero() {
		if elapsed := time.Since(room.State.UpdatedAt).Milliseconds(); elapsed > 0 {
			posMs += elapsed
		}
	}
	stateMsg := syncplay.SyncCommand{
		Type:       "state",
		Playing:    room.State.Playing,
		PositionMs: posMs,
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
// Bug #3: only the last connection for a user triggers member removal; while a
// second tab is open the member record stays and no member_left is emitted.
// Bug #2: host election is delegated to RemoveMemberAndDeleteIfEmpty, which
// performs it atomically under the repo lock so a concurrent disconnect cannot
// elect a member who has already departed.
func (h *Hub) DisconnectMember(ctx context.Context, roomID, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.syncplay.DisconnectMember")
	defer span.End()

	h.mu.Lock()
	var activeConns int
	if c := h.connCounts[roomID]; c != nil {
		c[userID]--
		activeConns = c[userID]
		if activeConns <= 0 {
			delete(c, userID)
			if m := h.conns[roomID]; m != nil {
				delete(m, userID)
				if len(m) == 0 {
					delete(h.conns, roomID)
				}
			}
			if len(c) == 0 {
				delete(h.connCounts, roomID)
			}
		}
	}
	h.mu.Unlock()

	if activeConns > 0 {
		// User still has other active connections in this room; member record
		// stays and no member_left is broadcast to avoid false departures.
		return nil
	}

	leaveMsg := syncplay.SyncCommand{
		Type:   "member_left",
		UserID: userID,
		RoomID: roomID,
	}
	h.broadcast(roomID, userID, leaveMsg)

	// Delete the room when this was the last member — otherwise a watch party
	// that ends by everyone closing their tab (the WS-close path, not the
	// LeaveRoom button) leaks its syncplay_rooms row forever.
	// Host election (if needed) is performed atomically inside the repo call.
	_, newHostID, err := h.repo.RemoveMemberAndDeleteIfEmpty(ctx, roomID, userID)
	if err != nil {
		return fmt.Errorf("disconnect member: %w", err)
	}

	if newHostID != "" {
		h.broadcast(roomID, "", syncplay.SyncCommand{
			Type:   "host_changed",
			UserID: newHostID,
			RoomID: roomID,
		})
	}

	return nil
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
		room, err := h.repo.Get(ctx, roomID)
		if err != nil || room == nil {
			return cmd, fmt.Errorf("get room: %w", err)
		}
		// Host-only: playback control (play/pause/seek) is driven by the host
		// so a guest can't desync or grief the watch party. The frontend
		// already hides these controls for non-hosts; this is the server-side
		// enforcement (a raw WebSocket client could otherwise send them).
		if string(room.HostID) != userID {
			return cmd, nil
		}
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
			state.Playing = room.State.Playing
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
		var idx int
		var mediaID string
		if cmd.Type == "setIndex" {
			// Absolute set: last-write-wins is fine, no read-modify-write race.
			idx = cmd.TrackIndex
			if idx < 0 || idx >= len(room.Queue) {
				return cmd, nil
			}
			mediaID = string(room.Queue[idx])
			if err := h.repo.UpdateTrackIndex(ctx, roomID, idx, mediaID); err != nil {
				return cmd, fmt.Errorf("update track index: %w", err)
			}
		} else {
			// next/prev: apply the delta atomically so two concurrent advances
			// (e.g. a host with two tabs) can't both read the same index and lose
			// an increment (+1 then +1 must land on +2, not +1).
			delta := 1
			if cmd.Type == "prev" {
				delta = -1
			}
			newIdx, mID, changed, advErr := h.repo.AdvanceTrackIndex(ctx, roomID, delta)
			if advErr != nil {
				return cmd, fmt.Errorf("advance track index: %w", advErr)
			}
			if !changed {
				return cmd, nil // out of bounds — no-op
			}
			idx, mediaID = newIdx, mID
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
		// Queue/track changes are authoritative server state EVERY client must
		// converge on — the host that sent it included. Unlike play/pause/seek
		// (where the sender already acted on its own audio element, so it is
		// skipped to avoid a feedback loop), the frontend does NOT advance the
		// queue locally on send: it waits for this echo. Broadcasting with an
		// empty sender skips no one, so the host actually moves to the next track.
		h.broadcast(roomID, "", cmd)
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
