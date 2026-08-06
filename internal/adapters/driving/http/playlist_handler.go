package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// playlistArchiveSource reads a playlist and its items. Both calls take the
// caller's profile id, which the service uses to enforce ownership.
type playlistArchiveSource interface {
	GetByID(ctx context.Context, id string, profileID shared.ProfileID) (*playlist.Playlist, error)
	GetItems(ctx context.Context, playlistID string, profileID shared.ProfileID) ([]*library.MediaItem, []*playlist.PlaylistItem, error)
}

// playlistGenerator builds a taste-weighted music playlist for a profile.
type playlistGenerator interface {
	Generate(ctx context.Context, profileID string, libraryID shared.LibraryID, size int, name string) (*playlist.Playlist, error)
}

// musicTrackResolver fetches a music track (its id lives in music_tracks, not
// media_items), used to add a playlist's music entries to the download ZIP.
type musicTrackResolver interface {
	GetTrack(ctx context.Context, id string) (*music.Track, error)
}

// playlistReorderer persists a new item order for a manual playlist. The
// service enforces ownership the same way GetByID/GetItems do (ErrNotOwner),
// so a caller can only reorder a playlist it owns.
type playlistReorderer interface {
	ReorderItems(ctx context.Context, playlistID string, profileID shared.ProfileID, mediaItemIDs []string) error
}

type playlistHandler struct {
	plSvc      playlistArchiveSource
	gen        playlistGenerator
	libSvc     MediaArchiveSource
	music      musicTrackResolver
	reorder    playlistReorderer
	libChecker LibraryAccessChecker
	parental   ParentalChecker
	baseURL    string
}

func registerPlaylistRoutes(mux *http.ServeMux, plSvc *appplaylist.Service, gen playlistGenerator, libSvc MediaArchiveSource, musicSvc musicTrackResolver, tokenGen domainuser.TokenGenerator, libChecker LibraryAccessChecker, parental ParentalChecker, baseURL string, rl *ratelimit.Limiter) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &playlistHandler{plSvc: plSvc, gen: gen, libSvc: libSvc, music: musicSvc, reorder: plSvc, libChecker: libChecker, parental: parental, baseURL: baseURL}
	// Like a collection ZIP, a playlist can fan out to many large files - a
	// heavier amplification vector than a single-file download, so rate-limit it.
	// streamAuth: opaque per-resource download token accepted alongside the
	// JWT (see download_token.go).
	streamAuthMw := httpStreamAuthMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/playlists/{id}/download", ratelimit.MiddlewareFunc(rl, streamAuthMw(h.downloadPlaylist)))
	// Generating scans the whole music library + the user's playlists - also
	// heavier than a normal call, so it shares the rate limiter.
	mux.HandleFunc("POST /api/v1/playlists/generate", ratelimit.MiddlewareFunc(rl, authMw(h.generatePlaylist)))
	// The proto only declares Add/Remove for playlist items - no reorder RPC -
	// so the mobile app's drag-to-reorder screen has 404'd since it shipped.
	// The domain already tracks item order (sort_order + a Reorder repo method
	// the CommandService exposes as ReorderItems), so this is a manual route
	// wiring an existing capability rather than new business logic.
	mux.HandleFunc("PUT /api/v1/playlists/{id}/items/reorder", authMw(h.reorderItems))
}

// reorderItemsRequest mirrors the mobile client's body: an array of
// {mediaItemId, sortOrder} pairs. sortOrder is authoritative (not array
// position) so a client that doesn't pre-sort still lands correctly.
type reorderItemsRequest struct {
	Items []struct {
		MediaItemID string `json:"mediaItemId"`
		SortOrder   int    `json:"sortOrder"`
	} `json:"items"`
}

// reorderItems persists a new item order for a playlist the caller owns.
// Body: {"items":[{"mediaItemId":"...","sortOrder":0}, ...]}.
func (h *playlistHandler) reorderItems(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.playlist.ReorderItems")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	playlistID := r.PathValue("id")
	span.SetAttributes(attribute.String("playlist_id", playlistID))

	var body reorderItemsRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.SetStatus(otelcodes.Error, "invalid request body")
		http.Error(w, `{"message":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	sort.SliceStable(body.Items, func(i, j int) bool { return body.Items[i].SortOrder < body.Items[j].SortOrder })
	ids := make([]string, 0, len(body.Items))
	for _, it := range body.Items {
		if it.MediaItemID == "" {
			continue
		}
		ids = append(ids, it.MediaItemID)
	}
	if len(ids) == 0 {
		span.SetStatus(otelcodes.Error, "empty items")
		http.Error(w, `{"message":"items is required"}`, http.StatusBadRequest)
		return
	}

	if err := h.reorder.ReorderItems(r.Context(), playlistID, shared.ProfileID(claims.ProfileID), ids); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplaylist.ErrForeignItems) || errors.Is(err, playlist.ErrReorderMembershipDrift) {
			// Same contract as collections: ids the playlist does not hold
			// are refused, not silently ignored - including an item that
			// left the playlist while the reorder was in flight.
			span.SetStatus(otelcodes.Error, "items contains non-members")
			http.Error(w, `{"message":"items contains entries that are not in this playlist"}`, http.StatusBadRequest)
			return
		}
		if errors.Is(err, appplaylist.ErrNotOwner) || errors.Is(err, shared.ErrNotFound) {
			// NotFound, not PermissionDenied: a playlist owned by another
			// profile must be indistinguishable from a non-existent one
			// (matches GetPlaylist/AddPlaylistItem/RemovePlaylistItem).
			span.SetStatus(otelcodes.Error, "playlist not found")
			http.Error(w, `{"message":"playlist not found"}`, http.StatusNotFound)
			return
		}
		span.SetStatus(otelcodes.Error, "reorder failed")
		slog.ErrorContext(r.Context(), "playlist reorder failed", slog.String("playlist_id", playlistID), slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	jsonEncodeResponse(w, map[string]string{"message": "reordered"})
}

// generatePlaylist builds and persists a taste-weighted music playlist for the
// caller. Body: {"libraryId":"...","size":50,"name":"..."}. The caller must
// have access to the library; the playlist is owned by the caller.
func (h *playlistHandler) generatePlaylist(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.playlist.Generate")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		LibraryID string `json:"libraryId"`
		Size      int    `json:"size"`
		Name      string `json:"name"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.LibraryID == "" {
		span.SetStatus(otelcodes.Error, "invalid request body")
		http.Error(w, `{"message":"libraryId is required"}`, http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(body.LibraryID), h.libChecker) {
		span.SetStatus(otelcodes.Error, "library access denied")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Mix"
	}

	pl, err := h.gen.Generate(r.Context(), claims.ProfileID, shared.LibraryID(body.LibraryID), body.Size, name)
	if errors.Is(err, appplaylist.ErrNoTracks) {
		span.SetStatus(otelcodes.Error, "no tracks")
		http.Error(w, `{"message":"no tracks to generate from"}`, http.StatusUnprocessableEntity)
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "generate failed")
		slog.ErrorContext(r.Context(), "playlist generate", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonEncodeResponse(w, map[string]string{"id": pl.ID, "name": pl.Name})
}

// downloadPlaylist streams a ZIP of a playlist's files. Ownership is enforced by
// the service (id + caller's user id), so a user can only download a playlist
// they created - a non-owner gets 404 (no existence leak). Works for both
// manual and smart playlists: for smart ones GetItems resolves the rules to the
// current matching set (a snapshot at download time). Files are confined to the
// library roots and access is re-checked per item (defense-in-depth).
func (h *playlistHandler) downloadPlaylist(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.playlist.Download")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	playlistID := r.PathValue("id")
	span.SetAttributes(attribute.String("playlist_id", playlistID))
	if !requireDownloadTokenBinding(w, r, playlistID) {
		return
	}

	var profileID shared.ProfileID
	if claims != nil {
		profileID = shared.ProfileID(claims.ProfileID)
	}

	pl, err := h.plSvc.GetByID(r.Context(), playlistID, profileID)
	if err != nil || pl == nil {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(otelcodes.Error, "playlist not found or not owned")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}

	mediaItems, plItems, err := h.plSvc.GetItems(r.Context(), playlistID, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list playlist items failed")
		slog.ErrorContext(r.Context(), "playlist download: get items", slog.String("playlist_id", playlistID), slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	canAccess := libraryAccessFunc(r.Context(), claims, h.libChecker)
	allowParental := parentalAllowFunc(r.Context(), claims, h.parental)
	roots := mediaRoots(r.Context(), h.libSvc)

	// Smart playlists hydrate to media items directly; manual playlists return
	// raw items (music tracks and/or media items) we resolve per type.
	var entries []archiveEntry
	entries = append(entries, buildMediaArchiveEntries(r.Context(), h.libSvc, mediaItems, canAccess, allowParental, roots)...)
	entries = append(entries, h.manualPlaylistEntries(r.Context(), plItems, canAccess, allowParental, roots)...)

	serveMediaArchive(w, r, pl.Name, entries, h.baseURL)
}

// manualPlaylistEntries turns a manual playlist's raw items into ZIP entries:
// music tracks (resolved from music_tracks) become single files; everything
// else is looked up in media_items and archived (movie/episode file, or a
// series/season folder). Inaccessible or out-of-root files are dropped.
func (h *playlistHandler) manualPlaylistEntries(ctx context.Context, plItems []*playlist.PlaylistItem, canAccess func(shared.LibraryID) bool, allowParental func(*library.MediaItem) bool, roots []string) []archiveEntry {
	var mediaIDs []string
	var musicEntries []archiveEntry
	for _, it := range plItems {
		if it == nil {
			continue
		}
		if it.MediaType == playlist.MediaTypeMusic {
			if h.music == nil {
				continue
			}
			tr, err := h.music.GetTrack(ctx, string(it.MediaItemID))
			if err != nil || tr == nil || tr.FilePath == "" {
				continue
			}
			if !canAccess(tr.LibraryID) || !pathWithinRoots(roots, tr.FilePath) {
				continue
			}
			musicEntries = append(musicEntries, archiveEntry{path: tr.FilePath, name: sanitizeFilename(tr.Title) + filepath.Ext(tr.FilePath)})
			continue
		}
		mediaIDs = append(mediaIDs, string(it.MediaItemID))
	}
	// Batch-fetch the media items (movies/episodes/series) in one query rather
	// than one GetMediaItem per playlist entry.
	var mediaItems []*library.MediaItem
	if len(mediaIDs) > 0 {
		items, err := h.libSvc.GetMediaItemsByIDs(ctx, mediaIDs)
		if err != nil {
			slog.WarnContext(ctx, "playlist download: batch get media items", slog.Any("err", err))
		} else {
			mediaItems = items
		}
	}
	entries := buildMediaArchiveEntries(ctx, h.libSvc, mediaItems, canAccess, allowParental, roots)
	return append(entries, musicEntries...)
}
