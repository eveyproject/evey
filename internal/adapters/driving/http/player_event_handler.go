package httpapi

// player_event_handler.go - the client-side playback error beacon.
//
// A playback failure the PLAYER sees (a decode error painting the screen in
// coloured macroblocks, an hls.js recovery loop, an endless buffer stall) can
// leave the server logs completely blank: every segment request answered 200,
// nothing to log. The 2026-07-15 incident was diagnosed by inference because
// of exactly that blindness. This endpoint lets the player post what it saw,
// as one WARN line per event - searchable next to the ffmpeg/session lines it
// belongs with (where=player, filtered by media_id).
//
// It is a telemetry door, so it trusts nothing: auth'd, rate-limited at the
// route, body capped, type whitelisted to [a-z0-9_], free text clamped.

import (
	"context"
	"encoding/json"
	domainplayback "github.com/evey-project/evey/internal/domain/playback"
	"log/slog"
	"net/http"
	"strings"

	"github.com/evey-project/evey/internal/app/presence"
	"github.com/evey-project/evey/internal/pkg/network"
)

const (
	// playerEventMaxBody bounds one beacon. The payload is a handful of short
	// fields; anything bigger is not a player talking.
	playerEventMaxBody = 2 << 10
	// playerEventMaxDetail clamps free-text fields going into the logs.
	playerEventMaxDetail = 200
)

type playerEventBody struct {
	// Type names the event family (hls_fatal, hls_media_error,
	// video_element_error, ...). Required, [a-z0-9_], <= 40 chars.
	Type string `json:"type"`
	// Details carries the player's own error detail string
	// (e.g. hls.js data.details "bufferAppendError").
	Details string `json:"details"`
	// State is the player's snapshot at the event (readyState, buffered range
	// around the playhead, dropped frames, tab visibility, wasm audio leg): the
	// same stall reads differently with an empty buffer than with a full one.
	State string `json:"state,omitempty"`
	Fatal bool   `json:"fatal"`
	// Code is whatever numeric code the player had: an HTTP status for network
	// errors, a MediaError.code for element-level ones.
	Code    int     `json:"code"`
	Variant int     `json:"variant"`
	Segment int     `json:"segment"`
	PosSec  float64 `json:"posSec"`
	// Paused is the cast receiver's own player state on a liveness beat. It
	// separates a television someone paused - a human act, and proof of life
	// even with the playhead frozen - from one that is merely stuck and still
	// believes it is playing.
	//
	// Absent on any other beacon, and on a receiver older than 2026-09-05,
	// where it decodes to false and the playhead alone decides. That fallback
	// is the safe one: a stall stays a stall.
	Paused bool `json:"paused,omitempty"`
}

// playerEventDeps carries the optional library/parental guard for
// playerEventHandler. Left zero-valued, the handler behaves exactly as
// before (no media_id ownership check) - this is a deliberate migration
// aid: the guard only activates once a caller wires it via
// WithPlayerEventLibraryGuard, so adding it here cannot regress the beacon
// route before its production registration (server.go) is updated to pass
// the dependencies.
type playerEventDeps struct {
	media      mediaLibraryGetter
	libChecker LibraryAccessChecker
	parental   ParentalChecker

	// Cast presence + TV-driven session keepalive (2026-07-29): during a cast
	// the PHONE carries ReportProgress, and an OEM freeze silences it - the
	// admin panel then loses the session (~2min reaper) and shows nobody
	// online while the TV plays on. The receiver's own beacons are the truth.
	castPresence    CastPresenceToucher
	resolveUsername func(ctx context.Context, userID string) string
	castSession     CastSessionTouch
	castSessionEnd  CastSessionEnd
	castCommand     CastCommandConsumer
	castAudioMode   CastAudioModeTouch
}

// CastAudioModeTouch mirrors the receiver's audio_mode beacon ("aac", "wasm",
// "aac-latched") onto the active session, so the admin badge follows the REAL
// audio path in near real time. Fire-and-forget by contract.
type CastAudioModeTouch func(ctx context.Context, profileID, mediaItemID, mode string)

// SessionServingRecord records the served path of the (profile, media)
// session at master render. Fire-and-forget by contract.
type SessionServingRecord func(ctx context.Context, profileID, mediaItemID string, p domainplayback.ServingPath)

// CastCommandConsumer hands the receiver the pending admin remote command of
// its session ("pause", "play", "stop", "message:<text>"), or "" when none.
// Destructive read - see playback.ConsumeCastCommand.
type CastCommandConsumer func(ctx context.Context, profileID, mediaItemID string) string

// CastPresenceToucher is the presence tracker's slice this handler needs.
type CastPresenceToucher interface {
	Touch(e presence.Entry)
}

// CastSessionTouch refreshes the active play session of (profile, media) from
// a receiver liveness beacon - position AND updated_at, so the stale-session
// reaper leaves a phone-orphaned cast alone. Fire-and-forget by contract.
//
// What the beat carries, and why each field matters, is on CastBeacon below.
type CastSessionTouch func(ctx context.Context, b CastBeacon)

// CastBeacon is one receiver liveness beat, as the session keepalive reads it.
// Mirrors playback.CastBeacon; cmd/evey maps between the two so this adapter
// keeps no dependency on the app package's types.
type CastBeacon struct {
	ProfileID   string
	MediaItemID string
	PositionMs  int64
	// AllowCreate: only cast_receiver_loaded may create a session.
	AllowCreate bool
	// Paused: the receiver reports a human paused it, which is proof of life
	// even with the playhead frozen.
	Paused bool
}

// CastSessionEnd ends the active play session of (profile, media) when the
// receiver announces its own death (cast_receiver_shutdown /
// cast_receiver_finished). Fire-and-forget by contract.
type CastSessionEnd func(ctx context.Context, profileID, mediaItemID string)

// PlayerEventOption configures playerEventHandler's optional dependencies.
type PlayerEventOption func(*playerEventDeps)

// WithPlayerEventCastPresence surfaces a beaconing cast receiver as an online
// DEVICE of the casting user ("EveyCast (Chromecast)" in the admin panel).
// resolveUsername maps the stream token's user id to a display name; nil or a
// blank result leaves the username empty.
func WithPlayerEventCastPresence(tr CastPresenceToucher, resolveUsername func(ctx context.Context, userID string) string) PlayerEventOption {
	return func(d *playerEventDeps) {
		d.castPresence = tr
		d.resolveUsername = resolveUsername
	}
}

// WithPlayerEventCastSession wires the TV-driven session keepalive.
func WithPlayerEventCastSession(touch CastSessionTouch) PlayerEventOption {
	return func(d *playerEventDeps) { d.castSession = touch }
}

// WithPlayerEventCastSessionEnd wires the receiver's end-of-life beacons to an
// immediate session end, instead of waiting on a silence the alive beat used to
// prevent.
func WithPlayerEventCastSessionEnd(end CastSessionEnd) PlayerEventOption {
	return func(d *playerEventDeps) { d.castSessionEnd = end }
}

// WithPlayerEventCastAudioMode wires the audio-path mirror.
func WithPlayerEventCastAudioMode(touch CastAudioModeTouch) PlayerEventOption {
	return func(d *playerEventDeps) { d.castAudioMode = touch }
}

// WithPlayerEventCastCommand makes the alive beacon's RESPONSE carry the
// pending admin command: the receiver polls every 30s with it, so the admin
// panel can pause/stop/message a cast whose phone is frozen.
func WithPlayerEventCastCommand(consume CastCommandConsumer) PlayerEventOption {
	return func(d *playerEventDeps) { d.castCommand = consume }
}

// WithPlayerEventLibraryGuard closes the "any JWT can push events onto an
// arbitrary media_id" gap: a JWT-authenticated caller must have library
// access to (and not be parental-blocked from) the media_id it reports on,
// exactly like the other handlers gated via checkLibraryAccess/checkParental.
// It does not apply to a stream-token caller (e.g. the cast receiver) -
// that token is already scoped to its own media_id, checked below.
func WithPlayerEventLibraryGuard(media mediaLibraryGetter, libChecker LibraryAccessChecker, parental ParentalChecker) PlayerEventOption {
	return func(d *playerEventDeps) {
		d.media = media
		d.libChecker = libChecker
		d.parental = parental
	}
}

// playerEventHandler accepts POST /api/v1/media/{id}/player-event and turns
// the body into one WARN log line. It never fails playback-side flows: the
// player fires and forgets.
func playerEventHandler(logger *slog.Logger, opts ...PlayerEventOption) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	var deps playerEventDeps
	for _, opt := range opts {
		opt(&deps)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		if mediaID == "" {
			http.Error(w, `{"message":"missing media id"}`, http.StatusBadRequest)
			return
		}
		// A stream token (the cast receiver's only identity - it has no user
		// JWT) is pinned to its own media: a leaked token must not become a
		// door to spray beacons onto arbitrary media ids. The token itself IS
		// the authorization here (same model as the syncplay guest path), so
		// it skips the claims-based library/parental guard below.
		if st, ok := StreamTokenFromContext(r.Context()); ok && st != nil {
			if string(st.MediaItemID) != mediaID {
				http.Error(w, `{"message":"token not valid for this media"}`, http.StatusForbidden)
				return
			}
		} else if deps.media != nil {
			claims := HTTPClaimsFromContext(r.Context())
			item, err := deps.media.GetMediaItem(r.Context(), mediaID)
			if err != nil || item == nil {
				// Mirror checkLibraryAccess's not-found response so an unknown
				// media_id and one the caller can't see are indistinguishable.
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, deps.libChecker) {
				return
			}
			if !checkParental(r.Context(), w, claims, item, deps.parental) {
				return
			}
		}
		var body playerEventBody
		r.Body = http.MaxBytesReader(w, r.Body, playerEventMaxBody)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"unparseable player event"}`, http.StatusBadRequest)
			return
		}
		if !validPlayerEventType(body.Type) {
			http.Error(w, `{"message":"type must be 1-40 chars of [a-z0-9_]"}`, http.StatusBadRequest)
			return
		}

		var userID string
		if claims := HTTPClaimsFromContext(r.Context()); claims != nil {
			userID = claims.UserID
		}
		notifyCastLiveness(r, &deps, mediaID, body.Type, body.PosSec, body.Paused)
		notifyCastAudioMode(r, &deps, mediaID, body.Type, body.Details)

		// Severity follows what the event IS. The route carries telemetry
		// (liveness beats every 30s, load/model/shutdown markers) alongside
		// genuine failures, and logging everything WARN drowned the admin log
		// view in fake errors - 27 on one healthy 10-minute cast (2026-07-28).
		// WARN is reserved for what the player flags fatal or an explicit
		// error/fatal type; everything else is INFO, same component, so the
		// where=player filter still shows the full story in one place.
		logLine := logger.Info
		msg := "player-client: playback event reported"
		// A receiver that stopped answering (cast_sender_stall) is an anomaly
		// worth the WARN level, like a fatal error: it is what an operator
		// searches for after "the phone kept spinning".
		if body.Fatal || strings.Contains(body.Type, "error") || strings.Contains(body.Type, "fatal") || strings.Contains(body.Type, "stall") {
			logLine = logger.Warn
			msg = "player-client: playback error reported"
		}
		logLine(msg,
			// The component drives the admin log search's where=player filter:
			// an operator investigating an incident finds the client's view of
			// it right next to the server's ffmpeg/session lines.
			slog.String("component", "player_client"),
			slog.String("media_id", mediaID),
			slog.String("user_id", userID),
			slog.String("type", body.Type),
			slog.String("details", clampString(body.Details, playerEventMaxDetail)),
			slog.String("state", clampString(body.State, playerEventMaxDetail)),
			slog.Bool("fatal", body.Fatal),
			slog.Int("code", body.Code),
			slog.Int("variant", body.Variant),
			slog.Int("seg", body.Segment),
			slog.Float64("pos_sec", body.PosSec),
			slog.String("device", clampString(r.UserAgent(), playerEventMaxDetail)),
		)
		// The alive beacon doubles as the TV's command poll: a pending admin
		// command rides back in the response body (200); the bare 204 stays
		// the answer everywhere else - including for the deployed receivers
		// that fire alive via sendBeacon and never read a response.
		if cmd := pendingCastCommand(r, &deps, mediaID, body.Type); cmd != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"command": cmd})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// castEndOfLifeEvents are the beacons the receiver sends as it dies. It stops
// its own heartbeat on each of them (MEDIA_FINISHED, SHUTDOWN, pagehide), so
// they are the earliest honest end of a cast - and the only one that ever
// arrives when the sender phone is already gone.
func isCastEndOfLife(eventType string) bool {
	return eventType == "cast_receiver_shutdown" || eventType == "cast_receiver_finished"
}

// notifyCastLiveness turns a receiver liveness beacon (loaded/alive, cast
// stream token only) into (1) a presence touch - the Chromecast shows up as an
// online device of the casting user - and (2) a play-session refresh, so the
// now-playing panel keeps tracking a cast whose phone froze. The phone's own
// JWT beacons are excluded - its RPCs cover it.
//
// End-of-life beacons END the session here. They used to touch nothing, on the
// reasoning that aging out was the point; but the alive beat made aging out
// impossible, so the receiver's own announcement of death was the one signal
// being thrown away. Presence is deliberately NOT touched for them: a dying
// receiver is not an online device.
func notifyCastLiveness(r *http.Request, deps *playerEventDeps, mediaID, eventType string, posSec float64, paused bool) {
	if isCastEndOfLife(eventType) {
		if deps.castSessionEnd == nil {
			return
		}
		st, ok := StreamTokenFromContext(r.Context())
		if !ok || st == nil || !st.Cast || st.UserID == "" {
			return
		}
		deps.castSessionEnd(r.Context(), string(st.ProfileID), mediaID)
		return
	}
	if eventType != "cast_receiver_loaded" && eventType != "cast_receiver_alive" {
		return
	}
	st, ok := StreamTokenFromContext(r.Context())
	if !ok || st == nil || !st.Cast || st.UserID == "" {
		return
	}
	if deps.castPresence != nil {
		username := ""
		if deps.resolveUsername != nil {
			username = deps.resolveUsername(r.Context(), string(st.UserID))
		}
		clientIP := ""
		if ip := network.ClientIP(r); ip != nil {
			clientIP = ip.String()
		}
		deps.castPresence.Touch(presence.Entry{
			UserID:    string(st.UserID),
			Username:  username,
			ProfileID: string(st.ProfileID),
			ClientIP:  clientIP,
			// A product UA the online panel maps to a TV chip, like the mobile
			// and TV apps' own UAs - the receiver's raw CrKey string would
			// render as a desktop Chrome on Linux.
			UserAgent: presence.CastReceiverUserAgent,
		})
	}
	if deps.castSession != nil && posSec > 0 {
		// Only a load may create a session; an alive beat may merely refresh one
		// - see CastBeacon.
		deps.castSession(r.Context(), CastBeacon{
			ProfileID:   string(st.ProfileID),
			MediaItemID: mediaID,
			PositionMs:  int64(posSec * 1000),
			AllowCreate: eventType == "cast_receiver_loaded",
			Paused:      paused,
		})
	}
}

// notifyCastAudioMode forwards an audio-path transition to the session row:
// the cast receiver's (cast stream token) or the web wasm player's (user JWT,
// event "wasm_audio_mode"). The mode string rides in Details.
func notifyCastAudioMode(r *http.Request, deps *playerEventDeps, mediaID, eventType, details string) {
	if deps.castAudioMode == nil {
		return
	}
	switch eventType {
	case "cast_receiver_audio_mode":
		st, ok := StreamTokenFromContext(r.Context())
		if !ok || st == nil || !st.Cast {
			return
		}
		deps.castAudioMode(r.Context(), string(st.ProfileID), mediaID, details)
	case "wasm_audio_mode":
		claims := HTTPClaimsFromContext(r.Context())
		if claims == nil || claims.ProfileID == "" {
			return
		}
		deps.castAudioMode(r.Context(), claims.ProfileID, mediaID, details)
	}
}

// pendingCastCommand consumes the session's remote command for a cast
// receiver's liveness beacon; every other caller gets "" (the phone's own
// RPC consume path already serves it).
func pendingCastCommand(r *http.Request, deps *playerEventDeps, mediaID, eventType string) string {
	if deps.castCommand == nil || (eventType != "cast_receiver_alive" && eventType != "cast_receiver_loaded") {
		return ""
	}
	st, ok := StreamTokenFromContext(r.Context())
	if !ok || st == nil || !st.Cast {
		return ""
	}
	return deps.castCommand(r.Context(), string(st.ProfileID), mediaID)
}

func validPlayerEventType(t string) bool {
	if len(t) == 0 || len(t) > 40 {
		return false
	}
	for _, c := range t {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

func clampString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
