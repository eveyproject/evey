package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

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
