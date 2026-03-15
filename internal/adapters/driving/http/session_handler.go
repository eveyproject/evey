package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	domainplayback "github.com/gravity-zero/evey/internal/domain/playback"
)

func activeSessionsHandler(tokenGen TokenVerifier, pbSvc interface {
	GetActiveSessions(ctx context.Context) ([]*domainplayback.ActiveSessionInfo, error)
}) http.HandlerFunc {
	type sessionJSON struct {
		SessionID     string  `json:"sessionId"`
		ProfileName   string  `json:"profileName"`
		MediaTitle    string  `json:"mediaTitle"`
		MediaType     string  `json:"mediaType"`
		MediaItemID   string  `json:"mediaItemId"`
		Progress      float64 `json:"progress"`
		DurationMs    int64   `json:"durationMs"`
		PositionMs    int64   `json:"positionMs"`
		StartedAt     string  `json:"startedAt"`
		DeviceName    string  `json:"deviceName"`
		DeviceType    string  `json:"deviceType"`
		ClientIP      string  `json:"clientIp"`
		PlayMethod    string  `json:"playMethod"`
		VideoCodec    string  `json:"videoCodec"`
		AudioCodec    string  `json:"audioCodec"`
		Resolution    string  `json:"resolution"`
		BandwidthBps  int64   `json:"bandwidthBps"`
		TranscodeInfo *string `json:"transcodeInfo,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		sessions, err := pbSvc.GetActiveSessions(r.Context())
		if err != nil {
			slog.Error("active sessions", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		out := make([]sessionJSON, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, sessionJSON{
				SessionID:     s.SessionID,
				ProfileName:   s.ProfileName,
				MediaTitle:    s.MediaTitle,
				MediaType:     s.MediaType,
				MediaItemID:   s.MediaItemID,
				Progress:      s.ProgressPercent(),
				DurationMs:    s.DurationMs,
				PositionMs:    s.PositionMs,
				StartedAt:     s.StartedAt.UTC().Format(time.RFC3339),
				DeviceName:    s.DeviceName,
				DeviceType:    s.DeviceType,
				ClientIP:      s.ClientIP,
				PlayMethod:    s.PlayMethod,
				VideoCodec:    s.VideoCodec,
				AudioCodec:    s.AudioCodec,
				Resolution:    s.Resolution,
				BandwidthBps:  s.BandwidthBps,
				TranscodeInfo: s.TranscodeInfo,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"sessions": out}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}

func killSessionHandler(tokenGen TokenVerifier, pbSvc interface {
	StopStream(ctx context.Context, sessionID string) error
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		sessionID := r.PathValue("id")
		if sessionID == "" {
			http.Error(w, `{"message":"session id required"}`, http.StatusBadRequest)
			return
		}
		if err := pbSvc.StopStream(r.Context(), sessionID); err != nil {
			slog.Error("kill session", slog.Any("err", err))
			http.Error(w, `{"message":"session not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
