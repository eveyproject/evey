package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	domaintranscoding "github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
	h.audit(r, "create_transcode_profile", "transcode_profile", &p.ID, map[string]any{"name": p.Name})
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
	h.audit(r, "update_transcode_profile", "transcode_profile", &p.ID, map[string]any{"name": p.Name})
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
	profileID := r.PathValue("id")
	if err := h.transcodeSvc.DeleteProfile(r.Context(), profileID); err != nil {
		slog.Error("delete transcode profile", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.audit(r, "delete_transcode_profile", "transcode_profile", &profileID, nil)
	w.WriteHeader(http.StatusNoContent)
}

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
	h.audit(r, "clear_transcode_cache", "transcode_cache", nil, nil)
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
