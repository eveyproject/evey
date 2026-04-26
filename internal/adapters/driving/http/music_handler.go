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

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appmusic "github.com/evey-project/evey/internal/app/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	_, span := tracing.StartSpan(r.Context(), "http.music.AlbumArt")
	defer span.End()
	hash := r.PathValue("hash")
	span.SetAttributes(attribute.String("album_art_hash", hash))
	if !albumArtHashRe.MatchString(hash) {
		span.SetStatus(otelcodes.Error, "invalid hash")
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
	ctx, span := tracing.StartSpan(r.Context(), "http.music.FetchMissingArt")
	defer span.End()
	r = r.WithContext(ctx)
	n, err := h.svc.FetchMissingAlbumArtAllLibraries(r.Context())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "fetch missing art failed")
		slog.Error("music: fetch missing art", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(map[string]int{"fetched": n}); encErr != nil {
		slog.Warn("json encode failed", slog.Any("err", encErr))
	}
}

func (h *musicHandler) getTrackArt(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.music.TrackArt")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	id := r.PathValue("id")
	span.SetAttributes(attribute.String("track_id", id))
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(otelcodes.Error, "track not found")
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		span.SetStatus(otelcodes.Error, "library access denied")
		return
	}
	data, mimeType, err := h.svc.GetTrackArt(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			span.SetStatus(otelcodes.Error, "art not found")
			http.NotFound(w, r)
			return
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "get track art failed")
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
	ctx, span := tracing.StartSpan(r.Context(), "http.music.StreamTrack")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	id := r.PathValue("id")
	span.SetAttributes(attribute.String("track_id", id))
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			span.SetStatus(otelcodes.Error, "track not found")
			http.NotFound(w, r)
			return
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "get track failed")
		slog.Error("music get track", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if track == nil {
		span.SetStatus(otelcodes.Error, "track not found")
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		span.SetStatus(otelcodes.Error, "library access denied")
		return
	}
	filePath, mimeType, err := h.svc.StreamTrack(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			span.SetStatus(otelcodes.Error, "stream track not found")
			http.NotFound(w, r)
			return
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "stream track failed")
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
	ctx, span := tracing.StartSpan(r.Context(), "http.music.DownloadTrack")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	id := r.PathValue("id")
	span.SetAttributes(attribute.String("track_id", id))
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(otelcodes.Error, "track not found")
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		span.SetStatus(otelcodes.Error, "library access denied")
		return
	}
	f, err := os.Open(track.FilePath)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "open track file failed")
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
	ctx, span := tracing.StartSpan(r.Context(), "http.music.DownloadAlbum")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	libraryID := r.URL.Query().Get("libraryId")
	album := r.URL.Query().Get("album")
	span.SetAttributes(
		attribute.String("library_id", libraryID),
		attribute.String("album", album),
	)
	if libraryID == "" || album == "" {
		span.SetStatus(otelcodes.Error, "libraryId and album required")
		http.Error(w, "libraryId and album are required", http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(libraryID), h.libChecker) {
		span.SetStatus(otelcodes.Error, "library access denied")
		return
	}
	tracks, err := h.svc.ListTracksByAlbum(r.Context(), shared.LibraryID(libraryID), album)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list tracks by album failed")
		slog.Error("music: list tracks by album", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(tracks) == 0 {
		span.SetStatus(otelcodes.Error, "no tracks for album")
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
