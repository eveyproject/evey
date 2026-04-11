package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	domainsyncplay "github.com/gravity-zero/evey/internal/domain/syncplay"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"golang.org/x/net/websocket"
)

type syncplayHandler struct {
	hub        *appsyncplay.Hub
	tokenGen   *auth.JWTGenerator
	media      mediaLibraryGetter
	libChecker LibraryAccessChecker
}

func registerSyncPlayRoutes(mux *http.ServeMux, hub *appsyncplay.Hub, tokenGen *auth.JWTGenerator, mediaGetter mediaLibraryGetter, libChecker LibraryAccessChecker) {
	h := &syncplayHandler{
		hub:        hub,
		tokenGen:   tokenGen,
		media:      mediaGetter,
		libChecker: libChecker,
	}
	// WebSocket uses Sec-WebSocket-Protocol for auth — can't use standard middleware
	mux.Handle("/api/v1/syncplay/rooms/{id}/ws", http.HandlerFunc(h.wsUpgradeHandler))
}

type wsCtxKey string

const (
	wsCtxUserID  wsCtxKey = "userID"
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

// wsSender adapts a websocket.Conn to the MessageSender port.
type wsSender struct {
	ws *websocket.Conn
}

func (s *wsSender) SendMessage(data []byte) error {
	_ = s.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := websocket.Message.Send(s.ws, string(data))
	_ = s.ws.SetWriteDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("wsSender.SendMessage: %w", err)
	}
	return nil
}

func (s *syncplayHandler) wsUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	token := extractWSToken(r)
	claims, vErr := s.tokenGen.ValidateAccessToken(token)
	if vErr != nil {
		slog.Warn("syncplay ws: invalid token", slog.String("room", roomID))
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	room, err := s.hub.GetRoom(r.Context(), roomID)
	if err != nil {
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	item, mErr := s.media.GetMediaItem(r.Context(), room.MediaID)
	if mErr != nil || item == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, s.libChecker) {
		return
	}
	if !room.IsMember(claims.UserID) {
		slog.Warn("syncplay ws: non-member attempt", slog.String("user", claims.UserID), slog.String("room", roomID))
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}

	ctx := context.WithValue(r.Context(), wsCtxUserID, claims.UserID)
	ctx = context.WithValue(ctx, wsCtxUsername, claims.Username)

	wsHandler := websocket.Handler(func(ws *websocket.Conn) {
		wsR := ws.Request()
		wsRoomID := wsR.PathValue("id")
		userID, _ := wsR.Context().Value(wsCtxUserID).(string)
		username, _ := wsR.Context().Value(wsCtxUsername).(string)
		wsCtx := wsR.Context()

		sender := &wsSender{ws: ws}
		_, connErr := s.hub.ConnectMember(wsCtx, wsRoomID, userID, username, sender)
		if connErr != nil {
			slog.Warn("syncplay ws: connect failed", slog.String("room", wsRoomID), slog.Any("err", connErr))
			_ = ws.Close()
			return
		}
		defer func() {
			if dErr := s.hub.DisconnectMember(wsCtx, wsRoomID, userID); dErr != nil {
				slog.Error("syncplay: disconnect", slog.String("room", wsRoomID), slog.Any("err", dErr))
			}
		}()

		const readDeadline = 60 * time.Second
		for {
			if rdErr := ws.SetReadDeadline(time.Now().Add(readDeadline)); rdErr != nil {
				break
			}
			var raw string
			if recvErr := websocket.Message.Receive(ws, &raw); recvErr != nil {
				break
			}
			var cmd domainsyncplay.SyncCommand
			if umErr := json.Unmarshal([]byte(raw), &cmd); umErr != nil {
				continue
			}
			if _, cmdErr := s.hub.HandleCommand(wsCtx, wsRoomID, userID, username, cmd); cmdErr != nil {
				slog.Error("syncplay: handle command", slog.String("room", wsRoomID), slog.Any("err", cmdErr))
			}
		}
	})
	wsHandler.ServeHTTP(w, r.WithContext(ctx))
}
