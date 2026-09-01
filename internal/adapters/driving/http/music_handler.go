package httpapi

import (
	"archive/zip"
	"context"
	"encoding/json"
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
	domainmusic "github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type musicHandler struct {
	svc          *appmusic.Service
	libChecker   LibraryAccessChecker
	libLister    MediaArchiveSource           // may be nil; used for path confinement (fail-open)
	imgConverter ImageConverter               // may be nil; ?w= resize is skipped when unset
	nowPlaying   *appmusic.NowPlayingRegistry // may be nil; "En cours de lecture" heartbeat
	// trackArtDir holds the cover bytes embedded in audio files once they have
	// been served with ?w=, so the resizer has a source PATH to work from
	// (album art resizes next to the folder cover; a track's art has no file
	// of its own). Empty = ?w= is ignored for track art and the tag bytes are
	// served in full, as before.
	trackArtDir string
}

// musicRouteOption tunes registerMusicRoutes without widening its signature
// for every test that builds the routes.
type musicRouteOption func(*musicHandler)

// withTrackArtCacheDir enables ?w= resizing of track (tag-embedded) art,
// caching the extracted originals and their variants under dir.
func withTrackArtCacheDir(dir string) musicRouteOption {
	return func(h *musicHandler) { h.trackArtDir = dir }
}

var albumArtHashRe = regexp.MustCompile(`^[a-f0-9]{16,128}$`)

// trackArtIDRe admits a track id as a cache FILE NAME: the id is the caller's
// (a route parameter), and the cache lives under a directory the server owns.
var trackArtIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func registerMusicRoutes(mux *http.ServeMux, svc *appmusic.Service, tokenGen domainuser.TokenGenerator, libChecker LibraryAccessChecker, _ libraryPathsGetter, rl *ratelimit.Limiter, libLister MediaArchiveSource, imgConverter ImageConverter, nowPlaying *appmusic.NowPlayingRegistry, opts ...musicRouteOption) {
	authMw := httpAuthMiddleware(tokenGen)
	adminMw := httpAdminMiddleware(tokenGen)
	h := &musicHandler{svc: svc, libChecker: libChecker, libLister: libLister, imgConverter: imgConverter, nowPlaying: nowPlaying, trackArtDir: ""}
	for _, o := range opts {
		o(h)
	}
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/art", authMw(h.getTrackArt))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", authMw(h.streamTrack))
	// streamAuth on the two download routes: opaque per-resource token
	// accepted alongside the JWT (see download_token.go).
	streamAuthMw := httpStreamAuthMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/download", streamAuthMw(ratelimit.MiddlewareFunc(rl, h.downloadTrack)))
	mux.HandleFunc("POST /api/v1/music/tracks/{id}/played", authMw(h.recordPlay))
	mux.HandleFunc("POST /api/v1/music/nowplaying", authMw(h.nowPlayingHeartbeat))
	mux.HandleFunc("GET /api/v1/music/albums/download", streamAuthMw(ratelimit.MiddlewareFunc(rl, h.downloadAlbum)))
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
	w.Header().Set("Cache-Control", "public, max-age=2592000")
	// ?w= is optional: a client that omits it (or an unparseable/out-of-bounds
	// value, snapped to 0 by parseRequestedWidth) keeps getting the original
	// full-res JPEG, same as before this endpoint supported resizing.
	if requestedWidth := parseRequestedWidth(r); requestedWidth > 0 && h.imgConverter != nil {
		if variantPath := h.resizeAlbumArt(r.Context(), path, requestedWidth); variantPath != "" {
			w.Header().Set("Content-Type", "image/webp")
			http.ServeFile(w, r, variantPath)
			return
		}
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, path)
}

// resizeAlbumArt returns the path to a cached (or freshly generated) WebP
// variant of src at width, reusing the exact cache/regenerate rules as
// artwork resizing (handleArtwork): reuse an existing non-empty variant,
// regenerate a missing/empty one under a bounded timeout, and never hand back
// a 0-byte file. Returns "" on any failure so the caller falls back to the
// full-res original.
func (h *musicHandler) resizeAlbumArt(ctx context.Context, src string, width int) string {
	variantPath := buildVariantPath(src, width)
	if fi, statErr := os.Stat(variantPath); statErr != nil || fi.Size() == 0 {
		ctx10, cancel := context.WithTimeout(ctx, artworkResizeTimeout)
		defer cancel()
		if resErr := h.imgConverter.ResizeAndConvertToWebP(ctx10, src, variantPath, width, 80); resErr != nil {
			slog.WarnContext(ctx, "album art resize", slog.String("src", src), slog.Int("width", width), slog.Any("err", resErr))
			removeVariant(variantPath)
			return ""
		}
	}
	if fi, statErr := os.Stat(variantPath); statErr != nil || fi.Size() == 0 {
		removeVariant(variantPath)
		return ""
	}
	return variantPath
}

func (h *musicHandler) fetchMissingArt(w http.ResponseWriter, r *http.Request) {
	// Scanning every album across all libraries plus external art fetches can far
	// outlast the proxy timeout on a large library; running it on the request ctx
	// means a 504 cancels the work mid-flight. Detach it (keeping trace/auth
	// values via WithoutCancel) and return 202 immediately - same intent as the
	// photo/metadata admin maintenance endpoints that run on a server context.
	bgCtx := context.WithoutCancel(r.Context())
	go func() {
		// A panic in a bare goroutine takes down the whole process; this
		// best-effort maintenance scan must never crash the server.
		defer func() {
			if rec := recover(); rec != nil {
				// bgCtx, not r.Context(): the request ctx is cancelled as soon
				// as the 202 goes out, which is the whole reason bgCtx exists.
				slog.ErrorContext(bgCtx, "music: fetch missing art panicked", slog.Any("panic", rec))
			}
		}()
		ctx, span := tracing.StartSpan(bgCtx, "http.music.FetchMissingArt")
		defer span.End()
		n, err := h.svc.FetchMissingAlbumArtAllLibraries(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "fetch missing art failed")
			slog.ErrorContext(ctx, "music: fetch missing art", slog.Any("err", err))
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
		slog.ErrorContext(r.Context(), "music get art", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// ?w= asks for a display-sized WebP, like album and poster art: a 48px
	// row thumbnail must not ship the full embedded cover (audit 2026-08-25).
	// Every failure falls through to the original bytes.
	if width := parseRequestedWidth(r); width > 0 {
		if variant := h.trackArtVariant(r.Context(), id, mimeType, data, width); variant != "" {
			w.Header().Set("Content-Type", "image/webp")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, variant)
			return
		}
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(data); err != nil {
		slog.ErrorContext(r.Context(), "music art write", slog.Any("err", err))
	}
}

// trackArtVariant returns the path of the WebP variant of a track's embedded
// cover at width, producing it on first request: the tag bytes are written
// once as trackArtDir/<id>.<ext> (the resizer works on a path), then resized
// with the same coalescing and timeout rules as album art. "" when the
// feature is off, the id is not a safe file name, or any step fails - the
// caller serves the original bytes then.
func (h *musicHandler) trackArtVariant(ctx context.Context, id, mimeType string, data []byte, width int) string {
	if h.trackArtDir == "" || h.imgConverter == nil || width <= 0 || len(data) == 0 || !trackArtIDRe.MatchString(id) {
		return ""
	}
	src := filepath.Join(h.trackArtDir, id+trackArtExt(mimeType))
	if fi, err := os.Stat(src); err != nil || fi.Size() == 0 {
		if err := os.MkdirAll(h.trackArtDir, 0o755); err != nil {
			slog.WarnContext(ctx, "track art cache dir", slog.String("dir", h.trackArtDir), slog.Any("err", err))
			return ""
		}
		tmp := src + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // server-owned cache dir
			slog.WarnContext(ctx, "track art cache write", slog.String("path", tmp), slog.Any("err", err))
			return ""
		}
		if err := os.Rename(tmp, src); err != nil {
			removeVariant(tmp)
			slog.WarnContext(ctx, "track art cache rename", slog.String("path", src), slog.Any("err", err))
			return ""
		}
	}
	return h.resizeAlbumArt(ctx, src, width)
}

// trackArtExt maps the tag's picture MIME type to the extension the resizer's
// ffmpeg probe expects; anything else is left generic, ffmpeg sniffs content.
func trackArtExt(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".img"
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
		slog.ErrorContext(r.Context(), "music get track", slog.Any("err", err))
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
		slog.ErrorContext(r.Context(), "music stream", slog.Any("err", err))
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
	// Disable write deadline for streaming - global WriteTimeout would cut large files.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.WarnContext(r.Context(), "set write deadline", slog.Any("err", err))
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
	if !requireDownloadTokenBinding(w, r, id) {
		return
	}
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
			slog.WarnContext(r.Context(), "music download: track path outside library roots, blocked",
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
		slog.WarnContext(r.Context(), "set write deadline", slog.Any("err", err))
	}
	http.ServeContent(w, r, track.FilePath, time.Time{}, f)
}

// recordPlay registers one listen of a track for the caller's profile, feeding
// the playlist generator's taste signal. Called by the player when a track
// finishes. Idempotency isn't a concern - each finished play is a real listen.
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
		// No active profile to attribute the play to - accept as a no-op.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.svc.RecordPlay(r.Context(), claims.ProfileID, id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "record play failed")
		slog.ErrorContext(r.Context(), "music: record play", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nowPlayingHeartbeat records the caller's live music playback so it shows in
// the admin "En cours de lecture" panel, and returns any queued remote command
// (play/pause/stop) the same way the video ProgressReporter receives one. Called
// every ~10s while a track plays, and once with stopped=true when playback ends.
func (h *musicHandler) nowPlayingHeartbeat(w http.ResponseWriter, r *http.Request) {
	if h.nowPlaying == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	claims := HTTPClaimsFromContext(r.Context())
	if claims == nil || claims.ProfileID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var body struct {
		SessionID  string `json:"sessionId"`
		TrackID    string `json:"trackId"`
		DeviceName string `json:"deviceName"`
		PositionMs int64  `json:"positionMs"`
		IsPaused   bool   `json:"isPaused"`
		Stopped    bool   `json:"stopped"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<11)).Decode(&body); err != nil || body.SessionID == "" {
		http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if body.Stopped {
		h.nowPlaying.Remove(body.SessionID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cmd := h.nowPlaying.Heartbeat(body.SessionID, claims.ProfileID, claims.Username, body.TrackID, body.DeviceName, body.PositionMs, body.IsPaused)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"remoteCommand": cmd})
}

func (h *musicHandler) downloadAlbum(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.music.DownloadAlbum")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	libraryID := r.URL.Query().Get("libraryId")
	albumID := r.URL.Query().Get("albumId")
	album := r.URL.Query().Get("album")
	span.SetAttributes(
		attribute.String("library_id", libraryID),
		attribute.String("album_id", albumID),
		attribute.String("album", album),
	)
	// The mint binds on the albumId when present, else the album NAME - the
	// same two spellings this endpoint accepts.
	if !requireDownloadTokenBinding(w, r, albumID+album) {
		return
	}
	if libraryID == "" || (albumID == "" && album == "") {
		span.SetStatus(otelcodes.Error, "libraryId and album/albumId required")
		http.Error(w, "libraryId and album (or albumId) are required", http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(libraryID), h.libChecker) {
		span.SetStatus(otelcodes.Error, "library access denied")
		return
	}
	// Prefer the opaque album id (collision-free, folder-derived); fall back to
	// the display name for older links.
	var tracks []domainmusic.Track
	var err error
	if albumID != "" {
		tracks, err = h.svc.ListTracksByAlbumID(r.Context(), shared.LibraryID(libraryID), albumID)
	} else {
		tracks, err = h.svc.ListTracksByAlbum(r.Context(), shared.LibraryID(libraryID), album)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list tracks by album failed")
		slog.ErrorContext(r.Context(), "music: list tracks by album", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(tracks) == 0 {
		span.SetStatus(otelcodes.Error, "no tracks for album")
		http.NotFound(w, r)
		return
	}
	// With an opaque id, the display name for the zip comes from the tracks.
	if album == "" {
		album = tracks[0].Album
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
		slog.WarnContext(r.Context(), "set write deadline", slog.Any("err", err))
	}
	zw := zip.NewWriter(w)
	defer func() {
		if closeErr := zw.Close(); closeErr != nil {
			slog.ErrorContext(r.Context(), "music: zip writer close failed",
				slog.String("album", album), slog.String("library_id", libraryID),
				slog.Any("err", closeErr))
		}
	}()
	seen := make(map[string]int)
	for _, tr := range tracks {
		// Defense-in-depth: the query already filtered by library_id, but assert
		// each track we're about to serve matches the access-checked library.
		if string(tr.LibraryID) != libraryID {
			slog.WarnContext(r.Context(), "music album download: cross-library track skipped",
				slog.String("track_id", tr.ID), slog.String("expected_lib", libraryID), slog.String("got_lib", string(tr.LibraryID)))
			continue
		}
		// Defense-in-depth: confine path to known library roots.
		if len(albumRoots) > 0 && !pathWithinRoots(albumRoots, tr.FilePath) {
			slog.WarnContext(r.Context(), "music album download: path outside library roots, skipped",
				slog.String("track_id", tr.ID), slog.String("path", tr.FilePath))
			continue
		}
		func() {
			f, openErr := os.Open(tr.FilePath)
			if openErr != nil {
				slog.WarnContext(r.Context(), "music album download: open", slog.String("path", tr.FilePath), slog.Any("err", openErr))
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
				slog.WarnContext(r.Context(), "music album download: zip create", slog.Any("err", zipErr))
				return
			}
			if _, copyErr := io.Copy(zfw, f); copyErr != nil {
				slog.WarnContext(r.Context(), "music album download: zip copy", slog.Any("err", copyErr))
			}
		}()
	}
}
