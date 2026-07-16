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
	"encoding/json"
	"log/slog"
	"net/http"
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
	Fatal   bool   `json:"fatal"`
	// Code is whatever numeric code the player had: an HTTP status for network
	// errors, a MediaError.code for element-level ones.
	Code    int     `json:"code"`
	Variant int     `json:"variant"`
	Segment int     `json:"segment"`
	PosSec  float64 `json:"posSec"`
}

// playerEventHandler accepts POST /api/v1/media/{id}/player-event and turns
// the body into one WARN log line. It never fails playback-side flows: the
// player fires and forgets.
func playerEventHandler(logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		if mediaID == "" {
			http.Error(w, `{"message":"missing media id"}`, http.StatusBadRequest)
			return
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
		logger.Warn("player-client: playback error reported",
			// The component drives the admin log search's where=player filter:
			// an operator investigating an incident finds the client's view of
			// it right next to the server's ffmpeg/session lines.
			slog.String("component", "player_client"),
			slog.String("media_id", mediaID),
			slog.String("user_id", userID),
			slog.String("type", body.Type),
			slog.String("details", clampString(body.Details, playerEventMaxDetail)),
			slog.Bool("fatal", body.Fatal),
			slog.Int("code", body.Code),
			slog.Int("variant", body.Variant),
			slog.Int("seg", body.Segment),
			slog.Float64("pos_sec", body.PosSec),
			slog.String("device", clampString(r.UserAgent(), playerEventMaxDetail)),
		)
		w.WriteHeader(http.StatusNoContent)
	}
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
