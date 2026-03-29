package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

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
