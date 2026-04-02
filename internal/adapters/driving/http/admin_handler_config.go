package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	goruntime "runtime"
	"time"
)

func (h *adminHandler) handleListServerConfig(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminWithResponse(w, r) {
		return
	}
	entries, err := h.cfgSvc.List(r.Context())
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
	if err := h.cfgSvc.Set(r.Context(), key, body.Value); err != nil {
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.audit(r, "set_server_config", "server_config", &key, map[string]any{"key": key})
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleDeleteServerConfig(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminWithResponse(w, r) {
		return
	}
	key := r.PathValue("key")
	if err := h.cfgSvc.Delete(r.Context(), key); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	h.audit(r, "delete_server_config", "server_config", &key, map[string]any{"key": key})
	w.WriteHeader(http.StatusNoContent)
}

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
	h.audit(r, "set_bitrate_limit", "user", &userID, map[string]any{"max_bitrate_kbps": body.MaxBitrateKbps})
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"max_bitrate_kbps": body.MaxBitrateKbps}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleEnrichPeople(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminWithResponse(w, r) {
		return
	}
	if h.bioEnricher == nil {
		http.Error(w, `{"message":"enricher not configured"}`, http.StatusServiceUnavailable)
		return
	}
	go func() {
		count, err := h.bioEnricher.EnrichAllPeopleBios(h.serverCtx, slog.Default())
		if err != nil {
			slog.Error("enrich people bios", slog.Any("err", err))
		} else {
			slog.Info("enrich people bios completed", slog.Int("enriched", count))
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"message":"enrichment started"}`))
}
