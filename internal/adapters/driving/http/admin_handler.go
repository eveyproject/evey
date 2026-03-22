package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	goruntime "runtime"
	"strconv"
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
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	claims, err := h.tokenGen.ValidateAccessToken(strings.TrimPrefix(header, "Bearer "))
	return err == nil && claims.IsAdmin
}

func (h *adminHandler) requireAdminWithResponse(w http.ResponseWriter, r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return false
	}
	claims, err := h.tokenGen.ValidateAccessToken(strings.TrimPrefix(header, "Bearer "))
	if err != nil || !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return false
	}
	return true
}

func (h *adminHandler) adminCheckWithQueryToken(r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	if tok == "" {
		return false
	}
	claims, err := h.tokenGen.ValidateAccessToken(tok)
	return err == nil && claims.IsAdmin
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

// --- user admin routes ---

func (h *adminHandler) handleGetBitrateLimit(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	userID := r.PathValue("id")
	limit, err := h.userQuery.GetUserBitrateLimit(r.Context(), userID)
	if err != nil {
		slog.Error("get user bitrate limit", slog.String("userID", userID), slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"max_bitrate_kbps": limit}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleSetBitrateLimit(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	userID := r.PathValue("id")
	var body struct {
		MaxBitrateKbps int `json:"max_bitrate_kbps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if body.MaxBitrateKbps < 0 {
		http.Error(w, `{"message":"max_bitrate_kbps must be >= 0"}`, http.StatusBadRequest)
		return
	}
	if err := h.userSvc.SetUserBitrateLimit(r.Context(), userID, body.MaxBitrateKbps); err != nil {
		slog.Error("set user bitrate limit", slog.String("userID", userID), slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"max_bitrate_kbps": body.MaxBitrateKbps}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

// --- logs routes ---

func (h *adminHandler) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	if !h.adminCheckWithQueryToken(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}

	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil && n > 0 && n <= 2000 {
			lines = n
		}
	}

	logPath := h.logDir + "/evey.log"
	data, err := tailFile(logPath, lines)
	if err != nil {
		http.Error(w, `{"message":"log file not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"lines": data}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	if !h.adminCheckWithQueryToken(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"message":"streaming unsupported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil && n >= 0 && n <= 2000 {
			lines = n
		}
	}

	logPath := h.logDir + "/evey.log"

	var lastSize int64
	if info, err := os.Stat(logPath); err == nil {
		lastSize = info.Size()
	}

	if lines > 0 {
		initial, tailErr := tailFile(logPath, lines)
		if tailErr != nil {
			slog.Warn("tail log file", slog.Any("err", tailErr))
		}
		for _, l := range initial {
			if l != "" {
				fmt.Fprintf(w, "data: %s\n\n", l)
			}
		}
		flusher.Flush()
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	const maxBytesPerTick = 64 * 1024

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			info, err := os.Stat(logPath)
			if err != nil {
				continue
			}
			newSize := info.Size()
			if newSize < lastSize {
				lastSize = 0
			}
			if newSize == lastSize {
				if _, writeErr := fmt.Fprintf(w, ": ping\n\n"); writeErr != nil {
					slog.Debug("sse client gone", slog.Any("err", writeErr))
					return
				}
				flusher.Flush()
				continue
			}

			f, err := os.Open(logPath)
			if err != nil {
				continue
			}
			if _, seekErr := f.Seek(lastSize, 0); seekErr != nil {
				f.Close()
				continue
			}
			buf := make([]byte, maxBytesPerTick)
			n, readErr := f.Read(buf)
			f.Close()
			if readErr != nil && n == 0 {
				continue
			}
			lastSize += int64(n)

			newLines := strings.Split(string(buf[:n]), "\n")
			for _, l := range newLines {
				if l != "" {
					if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", l); writeErr != nil {
						slog.Debug("sse client gone", slog.Any("err", writeErr))
						return
					}
				}
			}
			flusher.Flush()
		}
	}
}

// --- system stats ---

func (h *adminHandler) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	claims, err := h.tokenGen.ValidateAccessToken(strings.TrimPrefix(header, "Bearer "))
	if err != nil || !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
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

// --- server config ---

func (h *adminHandler) handleListServerConfig(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminWithResponse(w, r) {
		return
	}
	entries, err := h.cfgRepo.List(r.Context())
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type entry struct {
		Key       string    `json:"key"`
		Value     string    `json:"value"`
		UpdatedAt time.Time `json:"updatedAt"`
	}
	items := make([]entry, 0, len(entries))
	for _, e := range entries {
		items = append(items, entry{Key: e.Key, Value: e.Value, UpdatedAt: e.UpdatedAt})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"entries": items}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleSetServerConfig(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminWithResponse(w, r) {
		return
	}
	key := r.PathValue("key")
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if err := h.cfgRepo.Set(r.Context(), key, body.Value); err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleDeleteServerConfig(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminWithResponse(w, r) {
		return
	}
	key := r.PathValue("key")
	if err := h.cfgRepo.Delete(r.Context(), key); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// --- transcode cache ---

func (h *adminHandler) handleGetTranscodeCache(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	info, err := h.transcodeSvc.GetCacheInfo(r.Context())
	if err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type fileEntry struct {
		Name         string `json:"name"`
		Size         int64  `json:"size"`
		LastModified string `json:"lastModified"`
	}
	resp := struct {
		TotalSize int64       `json:"totalSize"`
		FileCount int         `json:"fileCount"`
		Files     []fileEntry `json:"files"`
	}{TotalSize: info.TotalSize, FileCount: info.FileCount, Files: make([]fileEntry, 0, len(info.Files))}
	for _, f := range info.Files {
		displayName := f.Name
		if h.mediaGetter != nil {
			if item, lookupErr := h.mediaGetter.GetMediaItem(r.Context(), f.Name); lookupErr == nil && item.Title != "" {
				displayName = item.Title
			}
		}
		resp.Files = append(resp.Files, fileEntry{Name: displayName, Size: f.Size, LastModified: f.LastModified.UTC().Format(time.RFC3339)})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleClearTranscodeCache(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	if err := h.transcodeSvc.ClearCache(r.Context()); err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleDeleteTranscodeCacheFile(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	filename := r.PathValue("filename")
	if err := h.transcodeSvc.DeleteCacheFile(r.Context(), filename); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			http.Error(w, `{"message":"invalid filename"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- register functions (preserve existing signatures) ---

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

// tailFile reads the last n lines from a file efficiently.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size == 0 {
		return nil, nil
	}

	readSize := min(int64(512*1024), size)
	offset := size - readSize
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, err
	}

	rawLines := strings.Split(string(buf), "\n")
	for len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	if len(rawLines) > n {
		rawLines = rawLines[len(rawLines)-n:]
	}
	return rawLines, nil
}
