package httpapi

import (
	"archive/zip"
	"context"
	"fmt"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appmusic "github.com/evey-project/evey/internal/app/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type musicHandler struct {
	svc        *appmusic.Service
	libChecker LibraryAccessChecker
	libLister  MediaArchiveSource // may be nil; used for path confinement (fail-open)
}

var albumArtHashRe = regexp.MustCompile(`^[a-f0-9]{16,128}$`)

func registerMusicRoutes(mux *http.ServeMux, svc *appmusic.Service, tokenGen domainuser.TokenGenerator, libChecker LibraryAccessChecker, _ libraryPathsGetter, rl *ratelimit.Limiter, libLister MediaArchiveSource) {
	authMw := httpAuthMiddleware(tokenGen)
	adminMw := httpAdminMiddleware(tokenGen)
	h := &musicHandler{svc: svc, libChecker: libChecker, libLister: libLister}
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/art", authMw(h.getTrackArt))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", authMw(h.streamTrack))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/download", authMw(ratelimit.MiddlewareFunc(rl, h.downloadTrack)))
	mux.HandleFunc("POST /api/v1/music/tracks/{id}/played", authMw(h.recordPlay))
	mux.HandleFunc("GET /api/v1/music/albums/download", authMw(ratelimit.MiddlewareFunc(rl, h.downloadAlbum)))
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
	// Scanning every album across all libraries plus external art fetches can far
	// outlast the proxy timeout on a large library; running it on the request ctx
	// means a 504 cancels the work mid-flight. Detach it (keeping trace/auth
	// values via WithoutCancel) and return 202 immediately — same intent as the
	// photo/metadata admin maintenance endpoints that run on a server context.
	bgCtx := context.WithoutCancel(r.Context())
	go func() {
		// A panic in a bare goroutine takes down the whole process; this
		// best-effort maintenance scan must never crash the server.
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("music: fetch missing art panicked", slog.Any("panic", rec))
			}
		}()
		ctx, span := tracing.StartSpan(bgCtx, "http.music.FetchMissingArt")
		defer span.End()
		n, err := h.svc.FetchMissingAlbumArtAllLibraries(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "fetch missing art failed")
			slog.Error("music: fetch missing art", slog.Any("err", err))
			return
		}
		slog.Info("music: fetched missing album art", slog.Int("count", n))
	}()
	w.WriteHeader(http.StatusAccepted)
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
	// Defense-in-depth: confine path to known library roots.
	if h.libLister != nil {
		roots := mediaRoots(r.Context(), h.libLister)
		if len(roots) > 0 && !pathWithinRoots(roots, track.FilePath) {
			slog.Warn("music download: track path outside library roots, blocked",
				slog.String("track_id", track.ID),
				slog.String("path", track.FilePath),
			)
			span.SetStatus(otelcodes.Error, "path outside library roots")
			http.NotFound(w, r)
			return
		}
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

// recordPlay registers one listen of a track for the caller's profile, feeding
// the playlist generator's taste signal. Called by the player when a track
// finishes. Idempotency isn't a concern — each finished play is a real listen.
func (h *musicHandler) recordPlay(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.music.RecordPlay")
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
	if claims == nil || claims.ProfileID == "" {
		// No active profile to attribute the play to — accept as a no-op.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.svc.RecordPlay(r.Context(), claims.ProfileID, id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "record play failed")
		slog.Error("music: record play", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	// Defense-in-depth: compute roots once for path confinement over the album's tracks.
	var albumRoots []string
	if h.libLister != nil {
		albumRoots = mediaRoots(r.Context(), h.libLister)
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
		// Defense-in-depth: confine path to known library roots.
		if len(albumRoots) > 0 && !pathWithinRoots(albumRoots, tr.FilePath) {
			slog.Warn("music album download: path outside library roots, skipped",
				slog.String("track_id", tr.ID), slog.String("path", tr.FilePath))
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
			// Dedup with a while-loop (same fix as serveMediaArchive): the
			// one-shot only bumped the ORIGINAL key and never registered the
			// renamed entry, so a track whose computed entryName equalled an
			// already-renamed file's name still collided.
			for {
				n, ok := seen[entryName]
				if !ok {
					seen[entryName] = 1
					break
				}
				extNow := filepath.Ext(entryName)
				baseNow := strings.TrimSuffix(entryName, extNow)
				seen[entryName] = n + 1
				entryName = fmt.Sprintf("%s (%d)%s", baseNow, n+1, extNow)
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
