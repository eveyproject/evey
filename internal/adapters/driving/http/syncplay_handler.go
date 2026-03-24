package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	domainsyncplay "github.com/gravity-zero/evey/internal/domain/syncplay"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"golang.org/x/net/websocket"
)

type syncplayHandler struct {
	hub        *appsyncplay.Hub
	tokenGen   *auth.JWTGenerator
	media      mediaLibraryGetter
	libChecker LibraryAccessChecker
	mu         sync.RWMutex
	conns      map[string][]*wsConn // roomID → connections
}

type wsConn struct {
	ws     *websocket.Conn
	userID string
}

func registerSyncPlayRoutes(mux *http.ServeMux, hub *appsyncplay.Hub, tokenGen *auth.JWTGenerator, mediaGetter mediaLibraryGetter, libChecker LibraryAccessChecker) {
	h := &syncplayHandler{
		hub:        hub,
		tokenGen:   tokenGen,
		media:      mediaGetter,
		libChecker: libChecker,
		conns:      make(map[string][]*wsConn),
	}
	mux.HandleFunc("POST /api/v1/syncplay/rooms", h.createRoom)
	mux.HandleFunc("POST /api/v1/syncplay/join", h.joinByCode)
	mux.HandleFunc("GET /api/v1/syncplay/rooms/{id}", h.getRoom)
	mux.HandleFunc("POST /api/v1/syncplay/rooms/{id}/join", h.joinRoom)
	mux.HandleFunc("DELETE /api/v1/syncplay/rooms/{id}/leave", h.leaveRoom)
	mux.Handle("/api/v1/syncplay/rooms/{id}/ws", h.wsHandler())
}

func (s *syncplayHandler) extractClaims(r *http.Request) (*domainuser.Claims, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, false
	}
	claims, err := s.tokenGen.ValidateAccessToken(h[7:])
	if err != nil {
		return nil, false
	}
	return claims, true
}

func (s *syncplayHandler) validateToken(token string) (userID, username string, err error) {
	claims, vErr := s.tokenGen.ValidateAccessToken(token)
	if vErr != nil {
		return "", "", vErr
	}
	return claims.UserID, claims.Username, nil
}

type wsCtxKey string

const (
	wsCtxUserID   wsCtxKey = "userID"
	wsCtxUsername wsCtxKey = "username"
)

func extractWSToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	for p := range strings.SplitSeq(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		p = strings.TrimSpace(p)
		if token, ok := strings.CutPrefix(p, "Bearer."); ok {
			return token
		}
	}
	return ""
}

func (s *syncplayHandler) wsHandler() http.Handler {
	wsHandler := websocket.Handler(func(ws *websocket.Conn) {
		r := ws.Request()
		roomID := r.PathValue("id")
		userID, _ := r.Context().Value(wsCtxUserID).(string)
		username, _ := r.Context().Value(wsCtxUsername).(string)

		ctx := r.Context()
		room, err := s.hub.ConnectMember(ctx, roomID, userID, username)
		if err != nil {
			slog.Warn("syncplay ws: connect failed", slog.String("room", roomID), slog.Any("err", err))
			_ = ws.Close()
			return
		}

		c := &wsConn{ws: ws, userID: userID}
		s.addConn(roomID, c)
		defer s.removeConn(roomID, c)

		// Send current state
		stateMsg := domainsyncplay.SyncCommand{
			Type:       "state",
			Playing:    room.State.Playing,
			PositionMs: room.State.PositionMs,
		}
		if data, mErr := json.Marshal(stateMsg); mErr == nil {
			_ = websocket.Message.Send(ws, string(data))
		}

		// Notify others
		joinMsg := domainsyncplay.SyncCommand{
			Type:     "member_joined",
			UserID:   userID,
			Username: domainsyncplay.MaskUsername(username),
			RoomID:   roomID,
		}
		s.broadcast(roomID, userID, joinMsg)

		const readDeadline = 60 * time.Second
		for {
			if err := ws.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
				break
			}
			var raw string
			if err := websocket.Message.Receive(ws, &raw); err != nil {
				break
			}
			var cmd domainsyncplay.SyncCommand
			if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
				continue
			}

			result, err := s.hub.HandleCommand(ctx, roomID, userID, username, cmd)
			if err != nil {
				slog.Error("syncplay: handle command", slog.String("room", roomID), slog.Any("err", err))
				continue
			}
			if cmd.Type == "play" || cmd.Type == "pause" || cmd.Type == "seek" {
				s.broadcast(roomID, userID, result)
			}
		}

		// On disconnect
		if err := s.hub.DisconnectMember(ctx, roomID, userID); err != nil {
			slog.Error("syncplay: disconnect", slog.String("room", roomID), slog.Any("err", err))
		}
		leaveMsg := domainsyncplay.SyncCommand{
			Type:   "member_left",
			UserID: userID,
			RoomID: roomID,
		}
		s.broadcast(roomID, userID, leaveMsg)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roomID := r.PathValue("id")
		token := extractWSToken(r)
		userID, username, err := s.validateToken(token)
		if err != nil {
			slog.Warn("syncplay ws: invalid token", slog.String("room", roomID))
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}

		room, err := s.hub.GetRoom(r.Context(), roomID)
		if err != nil {
			http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
			return
		}
		if !room.IsMember(userID) {
			slog.Warn("syncplay ws: non-member attempt", slog.String("user", userID), slog.String("room", roomID))
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), wsCtxUserID, userID)
		ctx = context.WithValue(ctx, wsCtxUsername, username)
		wsHandler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *syncplayHandler) addConn(roomID string, c *wsConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[roomID] = append(s.conns[roomID], c)
}

func (s *syncplayHandler) removeConn(roomID string, c *wsConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.conns[roomID]
	filtered := list[:0]
	for _, existing := range list {
		if existing != c {
			filtered = append(filtered, existing)
		}
	}
	s.conns[roomID] = filtered
}

func (s *syncplayHandler) broadcast(roomID, senderUserID string, cmd domainsyncplay.SyncCommand) {
	data, err := json.Marshal(cmd)
	if err != nil {
		return
	}
	s.mu.RLock()
	list := make([]*wsConn, len(s.conns[roomID]))
	copy(list, s.conns[roomID])
	s.mu.RUnlock()

	const sendTimeout = 5 * time.Second
	var wg sync.WaitGroup
	for _, c := range list {
		if c.userID == senderUserID {
			continue
		}
		wg.Add(1)
		go func(c *wsConn) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("syncplay: broadcast panic", slog.Any("recover", r), slog.String("user", c.userID))
				}
			}()
			_ = c.ws.SetWriteDeadline(time.Now().Add(sendTimeout))
			if err := websocket.Message.Send(c.ws, string(data)); err != nil {
				slog.Warn("syncplay: broadcast failed", slog.String("room", roomID), slog.String("user", c.userID), slog.Any("err", err))
			}
			_ = c.ws.SetWriteDeadline(time.Time{})
		}(c)
	}
	wg.Wait()
}

func (s *syncplayHandler) createRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		MediaID string `json:"mediaId"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MediaID == "" {
		http.Error(w, `{"message":"mediaId required"}`, http.StatusBadRequest)
		return
	}
	item, err := s.media.GetMediaItem(r.Context(), body.MediaID)
	if err != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, s.libChecker) {
		return
	}
	if body.Name == "" {
		body.Name = "Watch Together"
	}
	room, err := s.hub.CreateRoom(r.Context(), claims.UserID, body.MediaID, body.Name)
	if err != nil {
		slog.Error("syncplay create room", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":      room.ID,
		"code":    room.Code,
		"name":    room.Name,
		"mediaId": room.MediaID,
		"hostId":  room.HostID,
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (s *syncplayHandler) joinByCode(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		http.Error(w, `{"message":"code required"}`, http.StatusBadRequest)
		return
	}
	room, err := s.hub.GetRoomByCode(r.Context(), body.Code)
	if err != nil || room == nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	item, err := s.media.GetMediaItem(r.Context(), room.MediaID)
	if err != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, s.libChecker) {
		return
	}
	if err := s.hub.JoinRoom(r.Context(), room.ID, claims.UserID, claims.Username); err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"roomId":  room.ID,
		"mediaId": room.MediaID,
		"wsUrl":   "/api/v1/syncplay/rooms/" + room.ID + "/ws",
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (s *syncplayHandler) getRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	room, err := s.hub.GetRoom(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	isMember := room.HostID == claims.UserID
	for _, m := range room.Members {
		if m.UserID == claims.UserID {
			isMember = true
			break
		}
	}
	if !isMember && !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	type memberResp struct {
		UserID   string `json:"userId"`
		Username string `json:"username"`
	}
	members := make([]memberResp, 0, len(room.Members))
	for _, m := range room.Members {
		members = append(members, memberResp{UserID: m.UserID, Username: domainsyncplay.MaskUsername(m.Username)})
	}
	resp := map[string]any{
		"id":      room.ID,
		"name":    room.Name,
		"mediaId": room.MediaID,
		"hostId":  room.HostID,
		"members": members,
		"state": map[string]any{
			"playing":    room.State.Playing,
			"positionMs": room.State.PositionMs,
		},
	}
	if room.HostID == claims.UserID || claims.IsAdmin {
		resp["code"] = room.Code
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (s *syncplayHandler) joinRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	roomID := r.PathValue("id")
	room, err := s.hub.GetRoom(r.Context(), roomID)
	if err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	item, err := s.media.GetMediaItem(r.Context(), room.MediaID)
	if err != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, s.libChecker) {
		return
	}
	if err := s.hub.JoinRoom(r.Context(), roomID, claims.UserID, claims.Username); err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"roomId": roomID,
		"wsUrl":  "/api/v1/syncplay/rooms/" + roomID + "/ws",
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (s *syncplayHandler) leaveRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	if err := s.hub.LeaveRoom(r.Context(), r.PathValue("id"), claims.UserID); err != nil {
		slog.Warn("leave room", slog.String("room", r.PathValue("id")), slog.Any("err", err))
	}
	w.WriteHeader(http.StatusNoContent)
}
