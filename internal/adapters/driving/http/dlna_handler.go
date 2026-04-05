package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	appdlna "github.com/gravity-zero/evey/internal/app/dlna"
)

const dlnaMaxBodySize = 4096

func dlnaRenderersHandler(tokenGen TokenVerifier, dlnaSvc *appdlna.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := extractClaims(r, tokenGen)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		renderers, err := dlnaSvc.Discover(r.Context())
		if err != nil {
			slog.Error("dlna discover", slog.Any("err", err))
			http.Error(w, `{"message":"discovery failed"}`, http.StatusInternalServerError)
			return
		}
		if renderers == nil {
			renderers = []appdlna.Renderer{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(renderers); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}

func dlnaPlayHandler(tokenGen TokenVerifier, dlnaSvc *appdlna.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := extractClaims(r, tokenGen)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			RendererID string `json:"rendererId"`
			MediaURL   string `json:"mediaUrl"`
			Title      string `json:"title"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, dlnaMaxBodySize)).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.RendererID == "" || body.MediaURL == "" {
			http.Error(w, `{"message":"rendererId and mediaUrl required"}`, http.StatusBadRequest)
			return
		}
		if err := dlnaSvc.Play(r.Context(), body.RendererID, body.MediaURL, body.Title); err != nil {
			slog.Error("dlna play", slog.Any("err", err))
			http.Error(w, `{"message":"play failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func dlnaStopHandler(tokenGen TokenVerifier, dlnaSvc *appdlna.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := extractClaims(r, tokenGen)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			RendererID string `json:"rendererId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, dlnaMaxBodySize)).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.RendererID == "" {
			http.Error(w, `{"message":"rendererId required"}`, http.StatusBadRequest)
			return
		}
		if err := dlnaSvc.Stop(r.Context(), body.RendererID); err != nil {
			slog.Error("dlna stop", slog.Any("err", err))
			http.Error(w, `{"message":"stop failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func dlnaPauseHandler(tokenGen TokenVerifier, dlnaSvc *appdlna.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := extractClaims(r, tokenGen)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			RendererID string `json:"rendererId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, dlnaMaxBodySize)).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.RendererID == "" {
			http.Error(w, `{"message":"rendererId required"}`, http.StatusBadRequest)
			return
		}
		if err := dlnaSvc.Pause(r.Context(), body.RendererID); err != nil {
			slog.Error("dlna pause", slog.Any("err", err))
			http.Error(w, `{"message":"pause failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}
