package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"sync"

	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

const remoteAccessConfigKey = "remote_access"

// RemoteAccessConfigKey is the server config key for remote access settings.
const RemoteAccessConfigKey = remoteAccessConfigKey

func registerRemoteAccessRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen *auth.JWTGenerator, store remoteAccessPersister, writeRL *ratelimit.Limiter) {
	var mu sync.RWMutex
	admin := httpAdminMiddleware(tokenGen)

	encodeRA := func(w http.ResponseWriter) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"enabled":              cfg.RemoteAccess.Enabled,
			"externalUrl":          cfg.RemoteAccess.ExternalURL,
			"serverBaseUrl":        cfg.Server.BaseURL,
			"lanNetworks":          cfg.RemoteAccess.LANNetworks,
			"maxRemoteBitrateKbps": cfg.RemoteAccess.MaxRemoteBitrateKbps,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}

	mux.HandleFunc("GET /api/v1/admin/remote-access", admin(func(w http.ResponseWriter, r *http.Request) {
		encodeRA(w)
	}))

	mux.HandleFunc("PUT /api/v1/admin/remote-access", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled              *bool    `json:"enabled"`
			ExternalURL          *string  `json:"externalUrl"`
			LANNetworks          []string `json:"lanNetworks"`
			MaxRemoteBitrateKbps *int     `json:"maxRemoteBitrateKbps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		mu.Lock()
		if body.Enabled != nil {
			cfg.RemoteAccess.Enabled = *body.Enabled
		}
		if body.ExternalURL != nil {
			if *body.ExternalURL == "" {
				cfg.RemoteAccess.ExternalURL = ""
			} else {
				u, err := url.Parse(*body.ExternalURL)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					mu.Unlock()
					http.Error(w, `{"message":"invalid externalUrl: must be http/https URL"}`, http.StatusBadRequest)
					return
				}
				cfg.RemoteAccess.ExternalURL = *body.ExternalURL
			}
		}
		if body.LANNetworks != nil {
			cfg.RemoteAccess.LANNetworks = body.LANNetworks
		}
		if body.MaxRemoteBitrateKbps != nil {
			cfg.RemoteAccess.MaxRemoteBitrateKbps = *body.MaxRemoteBitrateKbps
		}
		snapshot := cfg.RemoteAccess
		mu.Unlock()

		if store != nil {
			if data, err := json.Marshal(snapshot); err == nil {
				if err2 := store.Set(r.Context(), remoteAccessConfigKey, string(data)); err2 != nil {
					slog.Warn("remote-access: persist config failed", slog.Any("err", err2))
				}
			}
		}
		encodeRA(w)
	})))

	mux.HandleFunc("GET /api/v1/admin/network-info", admin(func(w http.ResponseWriter, r *http.Request) {
		ifaces, err := network.NetworkInterfaces()
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"interfaces": ifaces}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))
}

func registerPlaybackSettingsRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen *auth.JWTGenerator, writeRL *ratelimit.Limiter) {
	authMw := httpAuthMiddleware(tokenGen)
	admin := httpAdminMiddleware(tokenGen)
	var mu sync.RWMutex

	mux.HandleFunc("GET /api/v1/playback-settings", authMw(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		fwd := cfg.Playback.SkipForwardSeconds
		bwd := cfg.Playback.SkipBackwardSeconds
		mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  fwd,
			"skipBackwardSeconds": bwd,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("PUT /api/v1/admin/playback-settings", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SkipForwardSeconds  *int `json:"skipForwardSeconds"`
			SkipBackwardSeconds *int `json:"skipBackwardSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		mu.Lock()
		if body.SkipForwardSeconds != nil && *body.SkipForwardSeconds > 0 {
			cfg.Playback.SkipForwardSeconds = *body.SkipForwardSeconds
		}
		if body.SkipBackwardSeconds != nil && *body.SkipBackwardSeconds > 0 {
			cfg.Playback.SkipBackwardSeconds = *body.SkipBackwardSeconds
		}
		fwd := cfg.Playback.SkipForwardSeconds
		bwd := cfg.Playback.SkipBackwardSeconds
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  fwd,
			"skipBackwardSeconds": bwd,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})))
}

