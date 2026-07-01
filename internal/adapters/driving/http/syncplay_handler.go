package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"golang.org/x/net/websocket"

	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	domainsyncplay "github.com/evey-project/evey/internal/domain/syncplay"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type syncplayHandler struct {
	hub        *appsyncplay.Hub
	tokenGen   domainuser.TokenGenerator
	media      mediaLibraryGetter
	libChecker LibraryAccessChecker
}

func registerSyncPlayRoutes(mux *http.ServeMux, hub *appsyncplay.Hub, tokenGen domainuser.TokenGenerator, mediaGetter mediaLibraryGetter, libChecker LibraryAccessChecker) {
	h := &syncplayHandler{
		hub:        hub,
		tokenGen:   tokenGen,
		media:      mediaGetter,
		libChecker: libChecker,
	}
	// WebSocket uses Sec-WebSocket-Protocol for auth — can't use standard middleware
	mux.Handle("/api/v1/syncplay/rooms/{id}/ws", http.HandlerFunc(h.wsUpgradeHandler))
}

// guestTokenMinter mints a streaming-only, media-scoped token for an
// unauthenticated syncplay guest. Satisfied by *media.StreamTokenStore.
type guestTokenMinter interface {
	CreateGuestToken(mediaItemID string, ttl time.Duration) string
}

// guestTokenTTL bounds an unauthenticated "watch together" guest session.
const guestTokenTTL = 24 * time.Hour

// syncplayJoinByCodeHandler is the PUBLIC (unauthenticated) entry point for a
// "watch together" guest: given a room code it mints a guest stream token scoped
// to the room's media so the guest can watch that one title, streaming-only.
// Must be rate-limited by the caller (brute-force of a 5-digit code). It reveals
// nothing beyond a generic 404 for an unknown/expired code.
func syncplayJoinByCodeHandler(hub *appsyncplay.Hub, tokens guestTokenMinter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.syncplay.JoinByCode")
		defer span.End()
		r = r.WithContext(ctx)

		var body struct {
			Code string `json:"code"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		code := strings.TrimSpace(body.Code)
		if len(code) < 4 || len(code) > 8 {
			http.Error(w, `{"message":"invalid code"}`, http.StatusBadRequest)
			return
		}

		room, err := hub.GetRoomByCode(ctx, code)
		if err != nil || room == nil {
			span.SetStatus(otelcodes.Error, "room not found")
			http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
			return
		}
		// Guest join is video-only: a music room's MediaID advances with the
		// queue, so a token pinned to one track would break on the next song.
		if room.MediaType == "music" {
			span.SetStatus(otelcodes.Error, "guest join unsupported for music rooms")
			http.Error(w, `{"message":"guest join is not available for music rooms"}`, http.StatusConflict)
			return
		}

		token := tokens.CreateGuestToken(string(room.MediaID), guestTokenTTL)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":     token,
			"roomId":    room.ID,
			"mediaId":   string(room.MediaID),
			"mediaType": room.MediaType,
			"code":      room.Code,
		})
	}
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

// wsSender adapts a websocket.Conn to the MessageSender port.
type wsSender struct {
	ws *websocket.Conn
	// mu serializes the SetWriteDeadline→Send→clear sequence. The hub can
	// broadcast to the same member from two goroutines at once (e.g. a host
	// command racing a member_joined fan-out). websocket.Message.Send already
	// serializes the frame write, but SetWriteDeadline touches the raw net.Conn
	// outside that lock — without this mutex one goroutine's "clear deadline"
	// could land while another's Send is in flight, leaving it to block forever
	// on a half-open socket and wedge every later write to that member.
	mu sync.Mutex
}

func (s *wsSender) SendMessage(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := websocket.Message.Send(s.ws, string(data))
	_ = s.ws.SetWriteDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("wsSender.SendMessage: %w", err)
	}
	return nil
}

func (s *syncplayHandler) wsUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	// Span only covers the upgrade handshake — the long-lived ws loop runs after.
	spanCtx, span := tracing.StartSpan(r.Context(), "http.syncplay.WSUpgrade")
	r = r.WithContext(spanCtx)
	roomID := r.PathValue("id")
	span.SetAttributes(attribute.String("room_id", roomID))
	token := extractWSToken(r)
	claims, vErr := s.tokenGen.ValidateAccessToken(token)
	if vErr != nil {
		span.RecordError(vErr)
		span.SetStatus(otelcodes.Error, "invalid token")
		span.End()
		slog.Warn("syncplay ws: invalid token", slog.String("room", roomID))
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	room, err := s.hub.GetRoom(r.Context(), roomID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "room not found")
		span.End()
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}
	// Music rooms carry a track ID in MediaID, which is not a media_item — the
	// video media/library gate below would 404 it. For music, access is gated at
	// create/join time (track library access) plus room membership (checked
	// below); the video media-item lookup is skipped.
	if room.MediaType != "music" {
		item, mErr := s.media.GetMediaItem(r.Context(), string(room.MediaID))
		if mErr != nil || item == nil {
			if mErr != nil {
				span.RecordError(mErr)
			}
			span.SetStatus(otelcodes.Error, "media item not found")
			span.End()
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, s.libChecker) {
			span.SetStatus(otelcodes.Error, "library access denied")
			span.End()
			return
		}
	}
	if !room.IsMember(claims.UserID) {
		span.SetStatus(otelcodes.Error, "non-member attempt")
		span.End()
		slog.Warn("syncplay ws: non-member attempt", slog.String("user", claims.UserID), slog.String("room", roomID))
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	// End the upgrade-span before entering the long-lived ws loop.
	span.End()

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
			if err := ws.Close(); err != nil {
				slog.Warn("syncplay ws close after connect failure", slog.Any("err", err))
			}
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
