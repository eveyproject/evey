package syncplay

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/syncplay"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"golang.org/x/net/websocket"
)

type conn struct {
	ws     *websocket.Conn
	userID string
	roomID string
}

type Hub struct {
	repo syncplay.RoomRepository

	mu    sync.RWMutex
	conns map[string][]*conn // roomID → connections
}

func NewHub(repo syncplay.RoomRepository) *Hub {
	return &Hub{
		repo:  repo,
		conns: make(map[string][]*conn),
	}
}

func (h *Hub) CreateRoom(ctx context.Context, hostID, mediaID, name string) (*syncplay.Room, error) {
	room := &syncplay.Room{
		ID:        id.New("room-"),
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
		return nil
	}
	if len(room.Members) == 0 {
		return h.repo.Delete(ctx, roomID)
	}
	return nil
}

// WSHandler returns an http.Handler for WebSocket connections at /api/v1/syncplay/rooms/{id}/ws
func (h *Hub) WSHandler(tokenValidator func(token string) (userID, username string, err error)) http.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		r := ws.Request()
		roomID := r.PathValue("id")

		token := r.URL.Query().Get("token")
		userID, username, err := tokenValidator(token)
		if err != nil {
			slog.Warn("syncplay ws: invalid token", slog.String("room", roomID))
			ws.Close()
			return
		}

		ctx := r.Context()
		room, err := h.repo.Get(ctx, roomID)
		if err != nil {
			slog.Warn("syncplay ws: room not found", slog.String("room", roomID))
			ws.Close()
			return
		}

		_ = h.repo.AddMember(ctx, roomID, syncplay.Member{
			UserID:   userID,
			Username: username,
			JoinedAt: time.Now().UTC(),
		})

		c := &conn{ws: ws, userID: userID, roomID: roomID}
		h.addConn(roomID, c)
		defer h.removeConn(roomID, c)

		// Send current state to new member
		stateMsg := syncplay.SyncCommand{
			Type:       "state",
			Playing:    room.State.Playing,
			PositionMs: room.State.PositionMs,
		}
		if data, err := json.Marshal(stateMsg); err == nil {
			_ = websocket.Message.Send(ws, string(data))
		}

		// Notify others
		joinMsg := syncplay.SyncCommand{
			Type:     "member_joined",
			UserID:   userID,
			Username: username,
			RoomID:   roomID,
		}
		h.broadcast(roomID, userID, joinMsg)

		// Read loop
		for {
			var raw string
			if err := websocket.Message.Receive(ws, &raw); err != nil {
				break
			}
			var cmd syncplay.SyncCommand
			if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
				continue
			}
			cmd.UserID = userID
			cmd.Username = username
			cmd.RoomID = roomID

			switch cmd.Type {
			case "play", "pause", "seek":
				state := syncplay.PlaybackState{
					Playing:   cmd.Type == "play",
					PositionMs: cmd.PositionMs,
					UpdatedAt: time.Now().UTC(),
					UpdatedBy: userID,
				}
				if cmd.Type == "pause" {
					state.Playing = false
				} else if cmd.Type == "seek" {
					// Keep existing playing state
					if existing, err := h.repo.Get(ctx, roomID); err == nil {
						state.Playing = existing.State.Playing
					}
				}
				_ = h.repo.UpdateState(ctx, roomID, state)
				h.broadcast(roomID, userID, cmd)
			}
		}

		// On disconnect
		_ = h.repo.RemoveMember(ctx, roomID, userID)
		leaveMsg := syncplay.SyncCommand{
			Type:   "member_left",
			UserID: userID,
			RoomID: roomID,
		}
		h.broadcast(roomID, userID, leaveMsg)
	})
}

func (h *Hub) addConn(roomID string, c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[roomID] = append(h.conns[roomID], c)
}

func (h *Hub) removeConn(roomID string, c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.conns[roomID]
	filtered := list[:0]
	for _, existing := range list {
		if existing != c {
			filtered = append(filtered, existing)
		}
	}
	h.conns[roomID] = filtered
}

func (h *Hub) broadcast(roomID, senderUserID string, cmd syncplay.SyncCommand) {
	data, err := json.Marshal(cmd)
	if err != nil {
		return
	}
	h.mu.RLock()
	list := make([]*conn, len(h.conns[roomID]))
	copy(list, h.conns[roomID])
	h.mu.RUnlock()

	for _, c := range list {
		if c.userID == senderUserID {
			continue
		}
		if err := websocket.Message.Send(c.ws, string(data)); err != nil {
			slog.Debug("syncplay broadcast send error", slog.Any("err", err))
		}
	}
}
