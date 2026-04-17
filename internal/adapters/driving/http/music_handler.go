package httpapi

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/auth"
)

type musicHandler struct {
	svc        *appmusic.Service
	libChecker LibraryAccessChecker
}

var albumArtHashRe = regexp.MustCompile(`^[a-f0-9]{16,128}$`)

func registerMusicRoutes(mux *http.ServeMux, svc *appmusic.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, _ libraryPathsGetter) {
	authMw := httpAuthMiddleware(tokenGen)
	adminMw := httpAdminMiddleware(tokenGen)
	h := &musicHandler{svc: svc, libChecker: libChecker}
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/art", authMw(h.getTrackArt))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", authMw(h.streamTrack))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/download", authMw(h.downloadTrack))
	mux.HandleFunc("GET /api/v1/music/albums/download", authMw(h.downloadAlbum))
	mux.HandleFunc("GET /api/v1/music/album-art/{hash}", authMw(h.serveAlbumArt))
	mux.HandleFunc("POST /api/v1/admin/music/fetch-missing-art", adminMw(h.fetchMissingArt))
}

func (h *musicHandler) serveAlbumArt(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !albumArtHashRe.MatchString(hash) {
		http.NotFound(w, r)
		return
	}
	dir := h.svc.AlbumArtDir()
	if dir == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(dir, hash+".jpg")
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	http.ServeFile(w, r, path)
}

func (h *musicHandler) fetchMissingArt(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.FetchMissingAlbumArtAllLibraries(r.Context())
	if err != nil {
		slog.Error("music: fetch missing art", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"fetched": n})
}

func (h *musicHandler) getTrackArt(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(track.LibraryID), h.libChecker) {
		return
	}
	data, mimeType, err := h.svc.GetTrackArt(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("music get art", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(data); err != nil {
		slog.Error("music art write", slog.Any("err", err))
	}
}

func (h *musicHandler) streamTrack(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
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
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(track.LibraryID), h.libChecker) {
		return
	}
	filePath, mimeType, err := h.svc.StreamTrack(r.Context(), id)
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
	// Disable write deadline for streaming — global WriteTimeout would cut large files.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("set write deadline", slog.Any("err", err))
	}
	http.ServeContent(w, r, filePath, time.Time{}, f)
}

func (h *musicHandler) downloadTrack(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(track.LibraryID), h.libChecker) {
		return
	}
	f, err := os.Open(track.FilePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	ext := filepath.Ext(track.FilePath)
	name := sanitizeFilename(track.Title) + ext
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("set write deadline", slog.Any("err", err))
	}
	http.ServeContent(w, r, track.FilePath, time.Time{}, f)
}

func (h *musicHandler) downloadAlbum(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
	libraryID := r.URL.Query().Get("libraryId")
	album := r.URL.Query().Get("album")
	if libraryID == "" || album == "" {
		http.Error(w, "libraryId and album are required", http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(libraryID), h.libChecker) {
		return
	}
	tracks, err := h.svc.ListTracksByAlbum(r.Context(), shared.LibraryID(libraryID), album)
	if err != nil {
		slog.Error("music: list tracks by album", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(tracks) == 0 {
		http.NotFound(w, r)
		return
	}
	zipName := sanitizeFilename(album) + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("set write deadline", slog.Any("err", err))
	}
	zw := zip.NewWriter(w)
	defer zw.Close()
	seen := make(map[string]int)
	for _, tr := range tracks {
		// Defense-in-depth: the query already filtered by library_id, but assert
		// each track we're about to serve matches the access-checked library.
		if string(tr.LibraryID) != libraryID {
			slog.Warn("music album download: cross-library track skipped",
				slog.String("track_id", tr.ID), slog.String("expected_lib", libraryID), slog.String("got_lib", string(tr.LibraryID)))
			continue
		}
		func() {
			f, openErr := os.Open(tr.FilePath)
			if openErr != nil {
				slog.Warn("music album download: open", slog.String("path", tr.FilePath), slog.Any("err", openErr))
				return
			}
			defer f.Close()
			ext := filepath.Ext(tr.FilePath)
			base := sanitizeFilename(tr.Title)
			if tr.TrackNumber > 0 {
				base = fmt.Sprintf("%02d - %s", tr.TrackNumber, base)
			}
			entryName := base + ext
			if n, ok := seen[entryName]; ok {
				entryName = fmt.Sprintf("%s (%d)%s", base, n+1, ext)
				seen[base+ext] = n + 1
			} else {
				seen[entryName] = 1
			}
			zfw, zipErr := zw.Create(entryName)
			if zipErr != nil {
				slog.Warn("music album download: zip create", slog.Any("err", zipErr))
				return
			}
			if _, copyErr := io.Copy(zfw, f); copyErr != nil {
				slog.Warn("music album download: zip copy", slog.Any("err", copyErr))
			}
		}()
	}
}
