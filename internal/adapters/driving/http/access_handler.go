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
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/network"
)

const remoteAccessConfigKey = "remote_access"

// RemoteAccessConfigKey is the server config key for remote access settings.
const RemoteAccessConfigKey = remoteAccessConfigKey

func registerLibraryAccessRoutes(mux *http.ServeMux, accessRepo domainuser.LibraryAccessRepository, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator) {
	requireAdminLocal := func(r *http.Request) bool {
		claims, ok := extractClaims(r, tokenGen)
		return ok && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/admin/users/{userId}/library-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		userID := r.PathValue("userId")
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
			ids = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"libraryIds": ids,
			"libraries":  allLibs,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("PUT /api/v1/admin/users/{userId}/library-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		userID := r.PathValue("userId")
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
			if err := accessRepo.Grant(r.Context(), userID, id); err != nil {
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/v1/admin/users/{userId}/library-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		userID := r.PathValue("userId")
		existing, _ := accessRepo.GetByUserID(r.Context(), userID) //nolint:errcheck
		for _, id := range existing {
			if rErr := accessRepo.Revoke(r.Context(), userID, id); rErr != nil {
				slog.Warn("revoke library access", slog.Any("err", rErr))
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func registerRemoteAccessRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen *auth.JWTGenerator, store remoteAccessPersister) {
	var mu sync.RWMutex

	requireAdminLocal := func(r *http.Request) bool {
		claims, ok := extractClaims(r, tokenGen)
		return ok && claims.IsAdmin
	}

	encodeRA := func(w http.ResponseWriter) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"enabled":              cfg.RemoteAccess.Enabled,
			"externalUrl":          cfg.RemoteAccess.ExternalURL,
			"lanNetworks":          cfg.RemoteAccess.LANNetworks,
			"maxRemoteBitrateKbps": cfg.RemoteAccess.MaxRemoteBitrateKbps,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}

	mux.HandleFunc("GET /api/v1/admin/remote-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		encodeRA(w)
	})

	mux.HandleFunc("PUT /api/v1/admin/remote-access", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
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
			u, err := url.Parse(*body.ExternalURL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				mu.Unlock()
				http.Error(w, `{"message":"invalid externalUrl: must be http/https URL"}`, http.StatusBadRequest)
				return
			}
			cfg.RemoteAccess.ExternalURL = *body.ExternalURL
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
	})

	mux.HandleFunc("GET /api/v1/admin/network-info", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		ifaces, err := network.NetworkInterfaces()
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"interfaces": ifaces}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})
}

func registerPlaybackSettingsRoutes(mux *http.ServeMux, cfg *config.Config, tokenGen *auth.JWTGenerator) {
	requireAuth := func(r *http.Request) bool {
		_, ok := extractClaims(r, tokenGen)
		return ok
	}
	requireAdminLocal := func(r *http.Request) bool {
		claims, ok := extractClaims(r, tokenGen)
		return ok && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/playback-settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  cfg.Playback.SkipForwardSeconds,
			"skipBackwardSeconds": cfg.Playback.SkipBackwardSeconds,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("PUT /api/v1/admin/playback-settings", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		var body struct {
			SkipForwardSeconds  *int `json:"skipForwardSeconds"`
			SkipBackwardSeconds *int `json:"skipBackwardSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.SkipForwardSeconds != nil && *body.SkipForwardSeconds > 0 {
			cfg.Playback.SkipForwardSeconds = *body.SkipForwardSeconds
		}
		if body.SkipBackwardSeconds != nil && *body.SkipBackwardSeconds > 0 {
			cfg.Playback.SkipBackwardSeconds = *body.SkipBackwardSeconds
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"skipForwardSeconds":  cfg.Playback.SkipForwardSeconds,
			"skipBackwardSeconds": cfg.Playback.SkipBackwardSeconds,
		}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})
}

func registerScheduleRoutes(mux *http.ServeMux, scheduler *applibrary.Scheduler, tokenGen *auth.JWTGenerator) {
	requireAdminLocal := func(r *http.Request) bool {
		claims, ok := extractClaims(r, tokenGen)
		return ok && claims.IsAdmin
	}

	mux.HandleFunc("GET /api/v1/libraries/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
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
	})

	mux.HandleFunc("PUT /api/v1/libraries/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
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
	})

	mux.HandleFunc("DELETE /api/v1/libraries/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminLocal(r) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if err := scheduler.DeleteSchedule(r.Context(), r.PathValue("id")); err != nil {
			slog.Warn("delete schedule", slog.Any("err", err))
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
