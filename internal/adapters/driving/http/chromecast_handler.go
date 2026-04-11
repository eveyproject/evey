package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	appcast "github.com/gravity-zero/evey/internal/app/chromecast"
)

const castMaxBodySize = 4096

func castDevicesHandler(tokenGen TokenVerifier, castSvc *appcast.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		devices, err := castSvc.Discover(r.Context())
		if err != nil {
			slog.Error("chromecast discover", slog.Any("err", err))
			http.Error(w, `{"message":"discovery failed"}`, http.StatusInternalServerError)
			return
		}
		if devices == nil {
			devices = []appcast.Device{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(devices); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}

func castPlayHandler(tokenGen TokenVerifier, castSvc *appcast.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DeviceUUID  string `json:"deviceUuid"`
			MediaURL    string `json:"mediaUrl"`
			Title       string `json:"title"`
			ContentType string `json:"contentType"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.DeviceUUID == "" || body.MediaURL == "" {
			http.Error(w, `{"message":"deviceUuid and mediaUrl required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Play(r.Context(), body.DeviceUUID, body.MediaURL, body.Title, body.ContentType); err != nil {
			slog.Error("chromecast play", slog.Any("err", err))
			http.Error(w, `{"message":"play failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castStopHandler(tokenGen TokenVerifier, castSvc *appcast.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DeviceUUID string `json:"deviceUuid"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.DeviceUUID == "" {
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Stop(r.Context(), body.DeviceUUID); err != nil {
			slog.Error("chromecast stop", slog.Any("err", err))
			http.Error(w, `{"message":"stop failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castPauseHandler(tokenGen TokenVerifier, castSvc *appcast.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DeviceUUID string `json:"deviceUuid"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.DeviceUUID == "" {
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Pause(r.Context(), body.DeviceUUID); err != nil {
			slog.Error("chromecast pause", slog.Any("err", err))
			http.Error(w, `{"message":"pause failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castSeekHandler(tokenGen TokenVerifier, castSvc *appcast.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DeviceUUID  string `json:"deviceUuid"`
			PositionSec int    `json:"positionSec"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.DeviceUUID == "" {
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Seek(r.Context(), body.DeviceUUID, body.PositionSec); err != nil {
			slog.Error("chromecast seek", slog.Any("err", err))
			http.Error(w, `{"message":"seek failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castStatusHandler(tokenGen TokenVerifier, castSvc *appcast.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := r.URL.Query().Get("deviceUuid")
		if uuid == "" {
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		status, err := castSvc.Status(r.Context(), uuid)
		if err != nil {
			slog.Error("chromecast status", slog.Any("err", err))
			http.Error(w, `{"message":"status failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}
