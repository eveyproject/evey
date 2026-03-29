package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/pkg/runtimedetect"
	domainshared "github.com/gravity-zero/evey/internal/domain/shared"
	domaintranscoding "github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

type adminHandler struct {
	tokenGen     *auth.JWTGenerator
	metaCmd      *appmetadata.CommandService
	metaQuery    *appmetadata.QueryService
	heavyRL      *ratelimit.Limiter
	userSvc      *appuser.UserService
	userQuery    *appuser.QueryService
	logDir       string
	startTime    time.Time
	cfgRepo      domainshared.ServerConfigRepository
	transcodeSvc *apptranscoding.Service
	mediaGetter  MediaItemGetter
	serverCtx    context.Context

	statsMu      sync.Mutex
	cachedMS     goruntime.MemStats
	cacheExpires time.Time
}

func (h *adminHandler) requireAdmin(r *http.Request) bool {
	_, err := extractAdminClaims(r, h.tokenGen)
	return err == nil
}

func (h *adminHandler) requireAdminWithResponse(w http.ResponseWriter, r *http.Request) bool {
	_, err := extractAdminClaims(r, h.tokenGen)
	if err == nil {
		return true
	}
	if errors.Is(err, errUnauthenticated) {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
	} else {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
	}
	return false
}

func (h *adminHandler) adminCheckWithQueryToken(r *http.Request) bool {
	_, err := extractAdminClaims(r, h.tokenGen)
	return err == nil
}

// --- media routes ---

func (h *adminHandler) handleMedia(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/media/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	mediaItemID := parts[0]
	action := parts[1]

	switch {
	case action == "refresh-metadata" && r.Method == http.MethodPost:
		h.handleRefreshMetadata(w, r, mediaItemID)
	case action == "search-metadata" && r.Method == http.MethodGet:
		h.handleSearchMetadata(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *adminHandler) handleRefreshMetadata(w http.ResponseWriter, r *http.Request, mediaItemID string) {
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	var body struct {
		TmdbID    string `json:"tmdb_id"`
		ImdbID    string `json:"imdb_id"`
		MediaType string `json:"media_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var refreshErr error
	if body.ImdbID != "" {
		refreshErr = h.metaCmd.RefreshItemMetadataByIMDB(r.Context(), mediaItemID, body.ImdbID, body.MediaType)
	} else {
		refreshErr = h.metaCmd.RefreshItemMetadata(r.Context(), mediaItemID, body.TmdbID, body.MediaType)
	}
	if refreshErr != nil {
		slog.Error("refresh metadata", slog.Any("err", refreshErr))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleSearchMetadata(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	mediaType := r.URL.Query().Get("type")
	if query == "" || mediaType == "" {
		http.Error(w, "query and type required", http.StatusBadRequest)
		return
	}
	results, err := h.metaQuery.SearchMetadata(r.Context(), query, mediaType)
	if err != nil {
		slog.Error("search metadata", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	type searchResult struct {
		TmdbID    string `json:"tmdb_id"`
		Title     string `json:"title"`
		Year      *int   `json:"year,omitempty"`
		Overview  string `json:"overview"`
		PosterURL string `json:"poster_url,omitempty"`
	}
	out := make([]searchResult, 0, len(results))
	for _, r := range results {
		sr := searchResult{
			Title:    r.Title,
			Year:     r.Year,
			Overview: r.Overview,
		}
		if tmdbID, ok := r.ProviderIDs["tmdb"]; ok {
			sr.TmdbID = tmdbID
		}
		for _, art := range r.Artwork {
			if art.Type == "poster" {
				sr.PosterURL = art.URL
				break
			}
		}
		out = append(out, sr)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleRefreshAll(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in bulk metadata refresh", slog.Any("error", r))
			}
		}()
		if err := h.metaCmd.AutoRefreshAllItems(h.serverCtx); err != nil {
			slog.Error("bulk metadata refresh", slog.Any("err", err))
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "refresh started"}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

// --- system stats ---

func (h *adminHandler) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminWithResponse(w, r) {
		return
	}

	h.statsMu.Lock()
	if time.Now().After(h.cacheExpires) {
		goruntime.ReadMemStats(&h.cachedMS)
		h.cacheExpires = time.Now().Add(30 * time.Second)
	}
	ms := h.cachedMS
	h.statsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"goroutines":    goruntime.NumGoroutine(),
		"memAllocBytes": ms.Alloc,
		"memSysBytes":   ms.Sys,
		"numGC":         ms.NumGC,
		"uptimeSeconds": int64(time.Since(h.startTime).Seconds()),
		"goVersion":     goruntime.Version(),
		"os":            goruntime.GOOS,
		"arch":          goruntime.GOARCH,
		"numCPU":        goruntime.NumCPU(),
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

// --- transcode profiles ---

type profileBody struct {
	Name         string `json:"name"`
	VideoCodec   string `json:"videoCodec"`
	AudioCodec   string `json:"audioCodec"`
	Container    string `json:"container"`
	MaxWidth     int    `json:"maxWidth"`
	MaxHeight    int    `json:"maxHeight"`
	VideoBitrate int64  `json:"videoBitrate"`
	AudioBitrate int64  `json:"audioBitrate"`
	Preset       string `json:"preset"`
}

func (h *adminHandler) handleCreateTranscodeProfile(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	var body profileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	p := &domaintranscoding.TranscodeProfile{
		ID:           id.New("tp-"),
		Name:         body.Name,
		VideoCodec:   body.VideoCodec,
		AudioCodec:   body.AudioCodec,
		Container:    body.Container,
		MaxWidth:     body.MaxWidth,
		MaxHeight:    body.MaxHeight,
		VideoBitrate: body.VideoBitrate,
		AudioBitrate: body.AudioBitrate,
		Preset:       body.Preset,
	}
	if err := h.transcodeSvc.CreateProfile(r.Context(), p); err != nil {
		slog.Error("create transcode profile", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleUpdateTranscodeProfile(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	profileID := r.PathValue("id")
	var body profileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	p := &domaintranscoding.TranscodeProfile{
		ID:           profileID,
		Name:         body.Name,
		VideoCodec:   body.VideoCodec,
		AudioCodec:   body.AudioCodec,
		Container:    body.Container,
		MaxWidth:     body.MaxWidth,
		MaxHeight:    body.MaxHeight,
		VideoBitrate: body.VideoBitrate,
		AudioBitrate: body.AudioBitrate,
		Preset:       body.Preset,
	}
	if err := h.transcodeSvc.UpdateProfile(r.Context(), p); err != nil {
		slog.Error("update transcode profile", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(p); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleDeleteTranscodeProfile(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	if err := h.transcodeSvc.DeleteProfile(r.Context(), r.PathValue("id")); err != nil {
		slog.Error("delete transcode profile", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- route registration ---

func registerAdminRoutes(mux *http.ServeMux, metaCmd *appmetadata.CommandService, metaQuery *appmetadata.QueryService, tokenGen *auth.JWTGenerator, heavyRL *ratelimit.Limiter, serverCtx context.Context) {
	h := &adminHandler{tokenGen: tokenGen, metaCmd: metaCmd, metaQuery: metaQuery, heavyRL: heavyRL, serverCtx: serverCtx}
	mux.HandleFunc("/api/v1/admin/media/", h.handleMedia)
	mux.HandleFunc("POST /api/v1/admin/metadata/refresh-all", h.handleRefreshAll)
}

func registerUserAdminRoutes(mux *http.ServeMux, userSvc *appuser.UserService, userQuery *appuser.QueryService, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, userSvc: userSvc, userQuery: userQuery}
	mux.HandleFunc("GET /api/v1/admin/users/{id}/bitrate-limit", h.handleGetBitrateLimit)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/bitrate-limit", h.handleSetBitrateLimit)
}

func registerLogsRoute(mux *http.ServeMux, logDir string, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, logDir: logDir}
	mux.HandleFunc("GET /api/v1/admin/logs", h.handleGetLogs)
	mux.HandleFunc("GET /api/v1/admin/logs/stream", h.handleStreamLogs)
}

func registerSystemStatsRoute(mux *http.ServeMux, startTime time.Time, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, startTime: startTime}
	mux.HandleFunc("GET /api/v1/admin/system/stats", h.handleSystemStats)
}

func registerServerConfigRoutes(mux *http.ServeMux, cfgRepo domainshared.ServerConfigRepository, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, cfgRepo: cfgRepo}
	mux.HandleFunc("GET /api/v1/admin/server-config", h.handleListServerConfig)
	mux.HandleFunc("PUT /api/v1/admin/server-config/{key}", h.handleSetServerConfig)
	mux.HandleFunc("DELETE /api/v1/admin/server-config/{key}", h.handleDeleteServerConfig)
}

func registerTranscodeProfileRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator) {
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc}
	mux.HandleFunc("POST /api/v1/admin/transcoding/profiles", h.handleCreateTranscodeProfile)
	mux.HandleFunc("PUT /api/v1/admin/transcoding/profiles/{id}", h.handleUpdateTranscodeProfile)
	mux.HandleFunc("DELETE /api/v1/admin/transcoding/profiles/{id}", h.handleDeleteTranscodeProfile)
}

func registerTranscodeCacheRoutes(mux *http.ServeMux, transcodeSvc *apptranscoding.Service, tokenGen *auth.JWTGenerator, mediaGetter MediaItemGetter) {
	h := &adminHandler{tokenGen: tokenGen, transcodeSvc: transcodeSvc, mediaGetter: mediaGetter}
	mux.HandleFunc("GET /api/v1/admin/transcode-cache", h.handleGetTranscodeCache)
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache", h.handleClearTranscodeCache)
	mux.HandleFunc("DELETE /api/v1/admin/transcode-cache/{filename}", h.handleDeleteTranscodeCacheFile)
}

func registerAvailableRuntimesRoute(mux *http.ServeMux, tokenGen *auth.JWTGenerator) {
	mux.HandleFunc("GET /api/v1/admin/plugin-runtimes/available", requireAdmin(tokenGen, func(w http.ResponseWriter, _ *http.Request) {
		runtimes := runtimedetect.DetectRuntimes()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"runtimes": runtimes}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))
}
