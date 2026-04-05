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
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
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
	// JSON routes migrated to gRPC — only WebSocket stays as custom HTTP
	mux.Handle("/api/v1/syncplay/rooms/{id}/ws", h.wsHandler())
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

func (s *syncplayHandler) wsHandler() http.Handler {
	wsHandler := websocket.Handler(func(ws *websocket.Conn) {
		r := ws.Request()
		roomID := r.PathValue("id")
		userID, _ := r.Context().Value(wsCtxUserID).(string)
		username, _ := r.Context().Value(wsCtxUsername).(string)
		ctx := r.Context()

		sender := &wsSender{ws: ws}
		_, err := s.hub.ConnectMember(ctx, roomID, userID, username, sender)
		if err != nil {
			slog.Warn("syncplay ws: connect failed", slog.String("room", roomID), slog.Any("err", err))
			_ = ws.Close()
			return
		}
		defer func() {
			if dErr := s.hub.DisconnectMember(ctx, roomID, userID); dErr != nil {
				slog.Error("syncplay: disconnect", slog.String("room", roomID), slog.Any("err", dErr))
			}
		}()

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
			if _, err := s.hub.HandleCommand(ctx, roomID, userID, username, cmd); err != nil {
				slog.Error("syncplay: handle command", slog.String("room", roomID), slog.Any("err", err))
			}
		}
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
		wsClaims, _ := s.tokenGen.ValidateAccessToken(token) //nolint:errcheck
		if wsClaims == nil {
			wsClaims = &domainuser.Claims{UserID: userID, Username: username}
		}
		item, mErr := s.media.GetMediaItem(r.Context(), room.MediaID)
		if mErr != nil || item == nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, wsClaims, item.LibraryID, s.libChecker) {
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
