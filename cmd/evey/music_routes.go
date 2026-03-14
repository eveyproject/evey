package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	appmusic "github.com/gravity-zero/evey/internal/app/music"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerMusicRoutes(mux *http.ServeMux, svc *appmusic.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker) {
	mux.HandleFunc("GET /api/v1/music/tracks", func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		libraryID := r.URL.Query().Get("library_id")
		if !checkLibraryAccess(r.Context(), w, claims, libraryID, libChecker) {
			return
		}
		limit := 50
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n >= 0 {
				offset = n
			}
		}
		tracks, total, err := svc.ListTracks(r.Context(), libraryID, limit, offset)
		if err != nil {
			slog.Error("music list tracks", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"tracks": tracks, "total": total}); err != nil {
			slog.Error("encode music tracks", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		track, err := svc.GetTrack(r.Context(), id)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			slog.Error("music get track", slog.Any("err", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if track == nil {
			http.NotFound(w, r)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, libChecker) {
			return
		}
		filePath, mimeType, err := svc.StreamTrack(r.Context(), id)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			slog.Error("music stream", slog.Any("err", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		f, err := os.Open(filePath)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer f.Close()
		if mimeType != "" {
			w.Header().Set("Content-Type", mimeType)
		}
		http.ServeContent(w, r, filePath, time.Time{}, f)
	})

	mux.HandleFunc("GET /api/v1/music/albums", func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		libraryID := r.URL.Query().Get("library_id")
		if !checkLibraryAccess(r.Context(), w, claims, libraryID, libChecker) {
			return
		}
		albums, err := svc.GetAlbums(r.Context(), libraryID)
		if err != nil {
			slog.Error("music list albums", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"albums": albums}); err != nil {
			slog.Error("encode music albums", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/music/artists", func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		libraryID := r.URL.Query().Get("library_id")
		if !checkLibraryAccess(r.Context(), w, claims, libraryID, libChecker) {
			return
		}
		artists, err := svc.GetArtists(r.Context(), libraryID)
		if err != nil {
			slog.Error("music list artists", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"artists": artists}); err != nil {
			slog.Error("encode music artists", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/music/search", func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		libraryID := r.URL.Query().Get("library_id")
		if !checkLibraryAccess(r.Context(), w, claims, libraryID, libChecker) {
			return
		}
		q := r.URL.Query().Get("q")
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		tracks, err := svc.Search(r.Context(), q, limit)
		if err != nil {
			slog.Error("music search", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"tracks": tracks}); err != nil {
			slog.Error("encode music search", slog.Any("err", err))
		}
	})

	mux.HandleFunc("POST /api/v1/music/scan", requireAdmin(tokenGen, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			LibraryID string `json:"library_id"`
			Dir       string `json:"dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		if body.LibraryID == "" || body.Dir == "" {
			http.Error(w, `{"message":"library_id and dir required"}`, http.StatusBadRequest)
			return
		}
		count, err := svc.ScanDirectory(r.Context(), body.LibraryID, body.Dir)
		if err != nil {
			slog.Error("music scan", slog.Any("err", err))
			http.Error(w, `{"message":"scan error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"scanned": count}); err != nil {
			slog.Error("encode music scan", slog.Any("err", err))
		}
	}))
}
