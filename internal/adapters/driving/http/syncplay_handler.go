package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"golang.org/x/net/websocket"

	appsyncplay "github.com/eveyproject/evey/internal/app/syncplay"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainsyncplay "github.com/eveyproject/evey/internal/domain/syncplay"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// guestTokenValidator resolves an opaque stream token. Satisfied by
// *media.StreamTokenStore. Used to authenticate a "watch together" guest on the
// sync WebSocket (a guest holds a stream token, not a JWT).
type guestTokenValidator interface {
	ValidateAny(token string) (*shared.StreamToken, bool)
}

type syncplayHandler struct {
	hub         *appsyncplay.Hub
	tokenGen    domainuser.TokenGenerator
	media       mediaLibraryGetter
	libChecker  LibraryAccessChecker
	guestTokens guestTokenValidator
	// parental gates a joining member the same way the HLS/byte/token stream
	// handlers do (see parental.go). May be nil - checkParental no-ops then.
	parental ParentalChecker
}

// syncPlayOption configures optional syncplayHandler dependencies. Added as a
// trailing variadic parameter so existing registerSyncPlayRoutes call sites
// keep compiling unchanged.
type syncPlayOption func(*syncplayHandler)

// withSyncPlayParental wires the parental gate onto the ws upgrade handler.
func withSyncPlayParental(pc ParentalChecker) syncPlayOption {
	return func(h *syncplayHandler) { h.parental = pc }
}

func registerSyncPlayRoutes(mux *http.ServeMux, hub *appsyncplay.Hub, tokenGen domainuser.TokenGenerator, mediaGetter mediaLibraryGetter, libChecker LibraryAccessChecker, guestTokens guestTokenValidator, opts ...syncPlayOption) {
	//nolint:exhaustruct_v5 // parental is deliberately absent here: it arrives through withSyncPlayParental, which server.go passes on the production call site.
	h := &syncplayHandler{
		hub:         hub,
		tokenGen:    tokenGen,
		media:       mediaGetter,
		libChecker:  libChecker,
		guestTokens: guestTokens,
	}
	for _, opt := range opts {
		opt(h)
	}
	// WebSocket uses Sec-WebSocket-Protocol for auth - can't use standard middleware
	mux.Handle("/api/v1/syncplay/rooms/{id}/ws", http.HandlerFunc(h.wsUpgradeHandler))
}

// guestTokenMinter mints a streaming-only, media-scoped token for an
// unauthenticated syncplay guest. Satisfied by *media.StreamTokenStore.
type guestTokenMinter interface {
	CreateGuestToken(mediaItemID, roomID string, ttl time.Duration) string
}

// guestTokenTTL bounds an unauthenticated "watch together" guest session. It is
// a SLIDING window (StreamTokenStore.Validate renews it on every fetch, up to
// the absolute 24 h cap), so this only has to outlast a pause, not a film - a
// session is one sitting, and a token nobody is using should not outlive the
// evening it was handed out for.
const guestTokenTTL = 3 * time.Hour

// Join-code failure gate. The per-IP limiter on this route bounds ONE source;
// the code space is global, so the counter that protects it has to be global
// too (see ratelimit.FailureGate). 200 wrong codes inside ten minutes is far
// beyond a household mistyping and far below what enumeration needs.
const (
	joinCodeFailureThreshold = 200
	joinCodeFailureWindow    = 10 * time.Minute
	joinCodeCooldown         = 5 * time.Minute
)

// syncplayJoinByCodeHandler is the PUBLIC (unauthenticated) entry point for a
// "watch together" guest: given a room code it mints a guest stream token scoped
// to the room's media so the guest can watch that one title, streaming-only.
// It reveals nothing beyond a generic 404 for an unknown/expired code.
//
// Two independent brakes, because this is the only unauthenticated mint in the
// product: the caller's per-IP limiter, and the global failure gate below that
// survives an attacker rotating source addresses.
func syncplayJoinByCodeHandler(hub *appsyncplay.Hub, tokens guestTokenMinter, gate *ratelimit.FailureGate) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !gate.Allow() {
			w.Header().Set("Retry-After", strconv.Itoa(int(joinCodeCooldown.Seconds())))
			http.Error(w, `{"message":"too many attempts, try again shortly"}`, http.StatusTooManyRequests)
			return
		}
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
			// A wrong code is the ONLY signal an enumeration produces, so it is
			// the only thing the gate counts - a malformed body is a client bug.
			gate.Fail()
			span.SetStatus(otelcodes.Error, "room not found")
			http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
			return
		}
		gate.Succeed()
		// Guest join is video-only: a music room's MediaID advances with the
		// queue, so a token pinned to one track would break on the next song.
		if room.MediaType == "music" {
			span.SetStatus(otelcodes.Error, "guest join unsupported for music rooms")
			http.Error(w, `{"message":"guest join is not available for music rooms"}`, http.StatusConflict)
			return
		}

		token := tokens.CreateGuestToken(string(room.MediaID), room.ID, guestTokenTTL)
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

// syncPlayMaxFrameBytes caps one inbound WebSocket frame. SyncPlay commands are
// short JSON control messages (type, position, index), so 8 KiB is generous
// while keeping a hostile client from making us buffer megabytes per message.
const syncPlayMaxFrameBytes = 8 << 10

// wsSender adapts a websocket.Conn to the MessageSender port.
type wsSender struct {
	ws *websocket.Conn
	// mu serializes the SetWriteDeadline→Send→clear sequence. The hub can
	// broadcast to the same member from two goroutines at once (e.g. a host
	// command racing a member_joined fan-out). websocket.Message.Send already
	// serializes the frame write, but SetWriteDeadline touches the raw net.Conn
	// outside that lock - without this mutex one goroutine's "clear deadline"
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

// guestMemberID derives a stable, non-reversible member id for a syncplay guest
// from their opaque token, so the token bytes are never exposed to other room
// members (member ids are broadcast). Same token → same id, so a reconnect maps
// back to the same guest.
func guestMemberID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "guest-" + hex.EncodeToString(sum[:6])
}

func (s *syncplayHandler) wsUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	// Span only covers the upgrade handshake - the long-lived ws loop runs after.
	spanCtx, span := tracing.StartSpan(r.Context(), "http.syncplay.WSUpgrade")
	r = r.WithContext(spanCtx)
	roomID := r.PathValue("id")
	span.SetAttributes(attribute.String("room_id", roomID))
	token := extractWSToken(r)
	claims, jwtErr := s.tokenGen.ValidateAccessToken(token)
	// A "watch together" guest holds a stream token, not a JWT. When the JWT is
	// invalid, only a guest-scoped stream token may proceed - reject anything else
	// here, BEFORE the room lookup, so an unauthenticated caller can't probe room
	// existence. The token is minted only by /syncplay/join-code (rate limited)
	// and is refused by the download handler.
	var guestTok *shared.StreamToken
	if jwtErr != nil {
		st, ok := s.guestTokens.ValidateAny(token)
		if !ok || st.Scope != shared.ScopeSyncplayGuest {
			span.RecordError(jwtErr)
			span.SetStatus(otelcodes.Error, "invalid token")
			span.End()
			slog.WarnContext(r.Context(), "syncplay ws: invalid token", slog.String("room", roomID))
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		guestTok = st
	}

	room, err := s.hub.GetRoom(r.Context(), roomID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "room not found")
		span.End()
		http.Error(w, `{"message":"room not found"}`, http.StatusNotFound)
		return
	}

	var wsUserID, wsUsername string
	if guestTok == nil {
		// Authenticated member path.
		wsUserID, wsUsername = claims.UserID, claims.Username
		// Music rooms carry a track ID in MediaID, not a media_item, so the video
		// media/library gate below would 404 it; music access is gated at
		// create/join time plus room membership (checked below).
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
			// Library access only proves the profile can see the LIBRARY; a kids
			// profile that learns a room id out-of-band (shared by the adult who
			// created it) must still be refused the specific adult title, same as
			// the HLS/byte/token stream handlers (checkParental).
			if !checkParental(r.Context(), w, claims, item, s.parental) {
				span.SetStatus(otelcodes.Error, "parental blocked")
				span.End()
				return
			}
		}
		if !room.IsMember(claims.UserID) {
			span.SetStatus(otelcodes.Error, "non-member attempt")
			span.End()
			slog.WarnContext(r.Context(), "syncplay ws: non-member attempt", slog.String("user", claims.UserID), slog.String("room", roomID))
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	} else {
		// Guest: the scoped token must belong to THIS room's media. The guest skips
		// the member and library checks - the room-media-scoped token IS the
		// authorization, and ConnectMember adds the guest as a member.
		if string(guestTok.MediaItemID) != string(room.MediaID) {
			span.SetStatus(otelcodes.Error, "guest token media mismatch")
			span.End()
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		wsUserID, wsUsername = guestMemberID(token), "Guest"
	}
	// End the upgrade-span before entering the long-lived ws loop.
	span.End()

	ctx := context.WithValue(r.Context(), wsCtxUserID, wsUserID)
	ctx = context.WithValue(ctx, wsCtxUsername, wsUsername)

	wsHandler := websocket.Handler(func(ws *websocket.Conn) {
		wsR := ws.Request()
		wsRoomID := wsR.PathValue("id")
		userID, _ := wsR.Context().Value(wsCtxUserID).(string)
		username, _ := wsR.Context().Value(wsCtxUsername).(string)
		wsCtx := wsR.Context()

		sender := &wsSender{ws: ws}
		_, connErr := s.hub.ConnectMember(wsCtx, wsRoomID, userID, username, sender)
		if connErr != nil {
			slog.WarnContext(r.Context(), "syncplay ws: connect failed", slog.String("room", wsRoomID), slog.Any("err", connErr))
			if err := ws.Close(); err != nil {
				slog.WarnContext(r.Context(), "syncplay ws close after connect failure", slog.Any("err", err))
			}
			return
		}
		defer func() {
			if dErr := s.hub.DisconnectMember(wsCtx, wsRoomID, userID); dErr != nil {
				slog.ErrorContext(r.Context(), "syncplay: disconnect", slog.String("room", wsRoomID), slog.Any("err", dErr))
			}
		}()

		const readDeadline = 60 * time.Second
		// A SyncCommand is a small JSON control message; x/net/websocket
		// otherwise allows a 32MB frame, so any connected member or guest could
		// make the server buffer and decode oversized payloads at will. The
		// read deadline bounds idle time, not size.
		ws.MaxPayloadBytes = syncPlayMaxFrameBytes
		unreadable := 0
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
				// Skipped, but said: a silently dropped command looks like a
				// room that does not follow its host. Once per connection: any
				// member, a guest included, could otherwise write a log line
				// per frame.
				unreadable++
				if unreadable == 1 {
					slog.WarnContext(r.Context(), "syncplay ws: unreadable command (further ones on this connection not logged)", slog.String("room", wsRoomID), slog.Any("err", umErr))
				}
				continue
			}
			if _, cmdErr := s.hub.HandleCommand(wsCtx, wsRoomID, userID, username, cmd); cmdErr != nil {
				slog.ErrorContext(r.Context(), "syncplay: handle command", slog.String("room", wsRoomID), slog.Any("err", cmdErr))
			}
		}
	})
	wsHandler.ServeHTTP(w, r.WithContext(ctx))
}
