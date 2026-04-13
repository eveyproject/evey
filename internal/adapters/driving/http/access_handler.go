package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/network"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

const remoteAccessConfigKey = "remote_access"

// RemoteAccessConfigKey is the server config key for remote access settings.
const RemoteAccessConfigKey = remoteAccessConfigKey

func registerLibraryAccessRoutes(mux *http.ServeMux, accessRepo domainuser.LibraryAccessRepository, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)

	mux.HandleFunc("GET /api/v1/admin/users/{userId}/library-access", admin(func(w http.ResponseWriter, r *http.Request) {
		userID := shared.UserID(r.PathValue("userId"))
		ids, err := accessRepo.GetByUserID(r.Context(), userID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		libs, libErr := libQuery.ListLibraries(r.Context())
		if libErr != nil {
			slog.Warn("list libraries for access", slog.Any("err", libErr))
		}
		type libInfo struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		allLibs := make([]libInfo, 0, len(libs))
		for _, l := range libs {
			allLibs = append(allLibs, libInfo{ID: l.ID, Name: l.Name})
		}
		if ids == nil {
			ids = []shared.LibraryID{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"libraryIds": ids,
			"libraries":  allLibs,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("PUT /api/v1/admin/users/{userId}/library-access", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		userID := shared.UserID(r.PathValue("userId"))
		var body struct {
			LibraryIDs []string `json:"libraryIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		existing, _ := accessRepo.GetByUserID(r.Context(), userID) //nolint:errcheck
		for _, id := range existing {
			if rErr := accessRepo.Revoke(r.Context(), userID, id); rErr != nil {
				slog.Warn("revoke library access", slog.Any("err", rErr))
			}
		}
		for _, id := range body.LibraryIDs {
			if err := accessRepo.Grant(r.Context(), userID, shared.LibraryID(id)); err != nil {
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	mux.HandleFunc("DELETE /api/v1/admin/users/{userId}/library-access", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		userID := shared.UserID(r.PathValue("userId"))
		existing, _ := accessRepo.GetByUserID(r.Context(), userID) //nolint:errcheck
		for _, id := range existing {
			if rErr := accessRepo.Revoke(r.Context(), userID, id); rErr != nil {
				slog.Warn("revoke library access", slog.Any("err", rErr))
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})))
}

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

func registerScheduleRoutes(mux *http.ServeMux, scheduler *applibrary.Scheduler, tokenGen *auth.JWTGenerator) {
	admin := httpAdminMiddleware(tokenGen)

	mux.HandleFunc("GET /api/v1/libraries/{id}/schedule", admin(func(w http.ResponseWriter, r *http.Request) {
		sched, err := scheduler.GetSchedule(r.Context(), r.PathValue("id"))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			if _, wErr := w.Write([]byte(`{"schedule":null}`)); wErr != nil {
				slog.Warn("write failed", slog.Any("err", wErr))
			}
			return
		}
		type resp struct {
			LibraryID string  `json:"libraryId"`
			Cron      string  `json:"cron"`
			Enabled   bool    `json:"enabled"`
			LastRunAt *string `json:"lastRunAt"`
			NextRunAt *string `json:"nextRunAt"`
		}
		out := resp{
			LibraryID: sched.LibraryID,
			Cron:      sched.CronExpression,
			Enabled:   sched.Enabled,
		}
		if sched.LastRunAt != nil {
			s := sched.LastRunAt.UTC().Format(time.RFC3339)
			out.LastRunAt = &s
		}
		if sched.NextRunAt != nil {
			s := sched.NextRunAt.UTC().Format(time.RFC3339)
			out.NextRunAt = &s
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"schedule": out}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("PUT /api/v1/libraries/{id}/schedule", admin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Cron    string `json:"cron"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.Cron == "" {
			http.Error(w, `{"message":"cron expression required"}`, http.StatusBadRequest)
			return
		}
		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}
		sched, err := scheduler.SetSchedule(r.Context(), r.PathValue("id"), body.Cron, enabled)
		if err != nil {
			if strings.HasPrefix(err.Error(), "invalid cron") {
				http.Error(w, `{"message":"invalid cron expression"}`, http.StatusBadRequest)
			} else {
				slog.Error("set schedule failed", slog.Any("err", err))
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			}
			return
		}
		type resp struct {
			LibraryID string  `json:"libraryId"`
			Cron      string  `json:"cron"`
			Enabled   bool    `json:"enabled"`
			NextRunAt *string `json:"nextRunAt"`
		}
		out := resp{LibraryID: sched.LibraryID, Cron: sched.CronExpression, Enabled: sched.Enabled}
		if sched.NextRunAt != nil {
			s := sched.NextRunAt.UTC().Format(time.RFC3339)
			out.NextRunAt = &s
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"schedule": out}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("DELETE /api/v1/libraries/{id}/schedule", admin(func(w http.ResponseWriter, r *http.Request) {
		if err := scheduler.DeleteSchedule(r.Context(), r.PathValue("id")); err != nil {
			slog.Warn("delete schedule", slog.Any("err", err))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
