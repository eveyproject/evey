package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

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
