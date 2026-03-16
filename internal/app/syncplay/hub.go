package syncplay

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/syncplay"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"golang.org/x/net/websocket"
)

type ctxKey string

const (
	ctxKeyUserID   ctxKey = "userID"
	ctxKeyUsername ctxKey = "username"
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
		return err
	}
	if len(room.Members) == 0 {
		return h.repo.Delete(ctx, roomID)
	}
	return nil
}

// extractWSToken reads the bearer token from the WebSocket upgrade request.
// Priority: Authorization header → Sec-WebSocket-Protocol subprotocol.
func extractWSToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// Browser WebSocket API cannot set custom headers; token is sent as a subprotocol.
	for p := range strings.SplitSeq(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		p = strings.TrimSpace(p)
		if token, ok := strings.CutPrefix(p, "Bearer."); ok {
			return token
		}
	}
	return ""
}

// WSHandler returns an http.Handler for WebSocket connections at /api/v1/syncplay/rooms/{id}/ws.
// Auth and membership checks happen before the WebSocket upgrade so HTTP 401/403 can be returned.
func (h *Hub) WSHandler(tokenValidator func(token string) (userID, username string, err error)) http.Handler {
	wsHandler := websocket.Handler(func(ws *websocket.Conn) {
		r := ws.Request()
		roomID := r.PathValue("id")
		// Claims injected by the pre-upgrade wrapper via context.
		userID, _ := r.Context().Value(ctxKeyUserID).(string)
		username, _ := r.Context().Value(ctxKeyUsername).(string)

		ctx := r.Context()
		room, err := h.repo.Get(ctx, roomID)
		if err != nil {
			slog.Warn("syncplay ws: room not found", slog.String("room", roomID))
			if err := ws.Close(); err != nil {
				slog.Warn("syncplay ws close", slog.Any("err", err))
			}
			return
		}

		if err := h.repo.AddMember(ctx, roomID, syncplay.Member{
			UserID:   userID,
			Username: username,
			JoinedAt: time.Now().UTC(),
		}); err != nil {
			slog.Error("syncplay: add member", slog.String("room", roomID), slog.Any("err", err))
		}

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
			if err := websocket.Message.Send(ws, string(data)); err != nil {
				slog.Warn("syncplay: send state", slog.String("room", roomID), slog.Any("err", err))
			}
		}

		// Notify others (mask username to avoid leaking login names)
		joinMsg := syncplay.SyncCommand{
			Type:     "member_joined",
			UserID:   userID,
			Username: syncplay.MaskUsername(username),
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
				switch cmd.Type {
				case "pause":
					state.Playing = false
				case "seek":
					if existing, getErr := h.repo.Get(ctx, roomID); getErr == nil {
						state.Playing = existing.State.Playing
					}
				}
				if err := h.repo.UpdateState(ctx, roomID, state); err != nil {
					slog.Error("syncplay: update state", slog.String("room", roomID), slog.Any("err", err))
				}
				h.broadcast(roomID, userID, cmd)
			}
		}

		// On disconnect
		if err := h.repo.RemoveMember(ctx, roomID, userID); err != nil {
			slog.Error("syncplay: remove member", slog.String("room", roomID), slog.Any("err", err))
		}
		leaveMsg := syncplay.SyncCommand{
			Type:   "member_left",
			UserID: userID,
			RoomID: roomID,
		}
		h.broadcast(roomID, userID, leaveMsg)
	})

	// Wrap wsHandler: validate token + membership before upgrading.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roomID := r.PathValue("id")
		token := extractWSToken(r)
		userID, username, err := tokenValidator(token)
		if err != nil {
			slog.Warn("syncplay ws: invalid token", slog.String("room", roomID))
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}

		room, err := h.repo.Get(r.Context(), roomID)
		if err != nil {
			http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
			return
		}
		if !room.IsMember(userID) {
			slog.Warn("syncplay ws: non-member attempt", slog.String("user", userID), slog.String("room", roomID))
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyUserID, userID)
		ctx = context.WithValue(ctx, ctxKeyUsername, username)
		wsHandler.ServeHTTP(w, r.WithContext(ctx))
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

	const sendTimeout = 5 * time.Second
	var wg sync.WaitGroup
	for _, c := range list {
		if c.userID == senderUserID {
			continue
		}
		wg.Add(1)
		go func(c *conn) {
			defer wg.Done()
			if err := c.ws.SetWriteDeadline(time.Now().Add(sendTimeout)); err != nil {
				slog.Warn("syncplay: set write deadline", slog.String("user", c.userID), slog.Any("err", err))
				return
			}
			if err := websocket.Message.Send(c.ws, string(data)); err != nil {
				slog.Warn("syncplay: broadcast send failed",
					slog.String("room", roomID), slog.String("user", c.userID), slog.Any("err", err))
			}
			if err := c.ws.SetWriteDeadline(time.Time{}); err != nil {
				slog.Warn("syncplay: reset write deadline", slog.String("user", c.userID), slog.Any("err", err))
			}
		}(c)
	}
	wg.Wait()
}
