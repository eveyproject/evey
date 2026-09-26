package grpcapi

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/shared"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type musicHandler struct {
	eveyv1.UnimplementedMusicServiceServer
	svc        *appmusic.Service
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
	serverCtx  context.Context
	translator *eveyi18n.Translator
}

// allowedLibraryIDs returns the library ids the caller may access, or nil for
// "all" (admins and users with no per-library restriction). Mirrors the library
// handler so cross-library music search can be access-filtered.
func (h *musicHandler) allowedLibraryIDs(ctx context.Context) (map[string]bool, error) {
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	if h.userAccess == nil {
		// nil reads as "all libraries" below (ListTracks/SearchMusic); a wiring
		// regression must deny instead, like a real zero-access user would get.
		return map[string]bool{}, nil
	}
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, internalError(ctx, "get library access", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

func (h *musicHandler) checkLibraryAccess(ctx context.Context, libraryID string) error {
	return libraryAccessOrStatus(ctx, h.translator, h.userAccess, libraryID, "musicHandler.checkLibraryAccess")
}

func (h *musicHandler) ListTracks(ctx context.Context, req *eveyv1.ListTracksRequest) (*eveyv1.ListTracksResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.music.ListTracks")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("musicHandler.ListTracks: %w", err)
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	claims := interceptors.ClaimsFromContext(ctx)
	isAdmin := claims != nil && claims.IsAdmin

	// Album-scoped listing: when album_id is set, return that album's full track
	// set (the opaque id resolves to a folder server-side), ignoring
	// limit/offset/search. Keeps homonym albums distinct and lets clients use an
	// opaque id in URLs instead of the display name.
	if req.AlbumId != "" {
		tracks, err := h.svc.ListTracksByAlbumID(ctx, shared.LibraryID(req.LibraryId), req.AlbumId)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "list tracks by album id", err)
		}
		resp := &eveyv1.ListTracksResponse{Total: int32(len(tracks))}
		for _, t := range tracks {
			resp.Tracks = append(resp.Tracks, trackToProto(t, isAdmin))
		}
		return resp, nil
	}

	limit, offset := clampLimitOffset(req.Limit, req.Offset, 50)

	if req.Shuffle {
		tracks, err := h.svc.QueryService().ShuffleTracks(ctx, shared.LibraryID(req.LibraryId), limit)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "shuffle tracks", err)
		}
		resp := &eveyv1.ListTracksResponse{Total: int32(len(tracks))}
		for _, t := range tracks {
			resp.Tracks = append(resp.Tracks, trackToProto(t, isAdmin))
		}
		return resp, nil
	}

	// Keyset path: the client already holds a cursor from a previous page, so
	// offset/total are skipped entirely - recomputing total here would pay
	// back exactly the COUNT(*) OVER() cost the cursor path exists to avoid.
	// A cursor is IGNORED while a free-text search is running: the search branch
	// below answers from the full-text index (typo tolerance, relevance), and
	// the keyset path can only filter with SQL. Honouring the cursor here would
	// serve page 1 from Meilisearch and page 2 from an ILIKE - the user would
	// watch their fuzzy match silently turn into a different result set
	// mid-scroll. Searches stay on one engine until the index has a keyset of
	// its own.
	if req.Cursor != "" && req.Search == "" {
		cursor, cErr := decodeTrackCursor(req.Cursor)
		if cErr != nil {
			span.RecordError(cErr)
			span.SetStatus(otelcodes.Error, "invalid cursor")
			slog.WarnContext(ctx, "grpc ListTracks: invalid cursor", slog.Any("err", cErr))
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.cursor_invalid")
		}
		tracks, err := h.svc.QueryService().ListTracksCursor(ctx, shared.LibraryID(req.LibraryId), req.Search, cursor, limit)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "list tracks", err)
		}
		resp := &eveyv1.ListTracksResponse{}
		for _, t := range tracks {
			resp.Tracks = append(resp.Tracks, trackToProto(t, isAdmin))
		}
		if len(tracks) >= limit {
			resp.NextCursor = encodeTrackCursor(tracks[len(tracks)-1])
		}
		return resp, nil
	}

	// Free-text search goes through the full-text index (Meilisearch, with SQL
	// fallback) for typo tolerance and relevance, instead of a raw SQL ILIKE.
	if req.Search != "" {
		tracks, err := h.svc.Search(ctx, shared.LibraryID(req.LibraryId), req.Search, limit)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "search tracks", err)
		}
		resp := &eveyv1.ListTracksResponse{Total: int32(len(tracks))}
		for _, t := range tracks {
			resp.Tracks = append(resp.Tracks, trackToProto(t, isAdmin))
		}
		return resp, nil
	}

	tracks, total, err := h.svc.ListTracks(ctx, shared.LibraryID(req.LibraryId), limit, offset, req.Search)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list tracks", err)
	}
	resp := &eveyv1.ListTracksResponse{Total: int32(total)}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, trackToProto(t, isAdmin))
	}
	// Hand out the first cursor from the offset path too, otherwise keyset
	// paging is unreachable: a client can only send a cursor it was given, and
	// with page 1 refusing to produce one, nobody would ever leave the offset
	// path. Page 1 keeps its exact total, every page after it is served by
	// index seek. Clients that ignore the field are unaffected.
	if len(tracks) >= limit && req.Search == "" {
		resp.NextCursor = encodeTrackCursor(tracks[len(tracks)-1])
	}
	return resp, nil
}

func (h *musicHandler) GetTrack(ctx context.Context, req *eveyv1.GetTrackRequest) (*eveyv1.GetTrackResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.music.GetTrack")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("musicHandler.GetTrack: %w", err)
	}
	track, err := h.svc.GetTrack(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get track", err)
	}
	if track == nil {
		span.SetStatus(otelcodes.Error, "track not found")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.music.track_not_found")
	}
	// Access check AFTER resolving the track: the caller supplies only an id, so
	// the track's own library decides whether they may see it (don't leak tracks
	// from libraries the user can't access).
	if err := h.checkLibraryAccess(ctx, string(track.LibraryID)); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	claims := interceptors.ClaimsFromContext(ctx)
	isAdmin := claims != nil && claims.IsAdmin
	return &eveyv1.GetTrackResponse{Track: trackToProto(*track, isAdmin)}, nil
}

func (h *musicHandler) GetAlbums(ctx context.Context, req *eveyv1.GetAlbumsRequest) (*eveyv1.GetAlbumsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.music.GetAlbums")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("musicHandler.GetAlbums: %w", err)
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	albums, err := h.svc.GetAlbums(ctx, shared.LibraryID(req.LibraryId))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get albums", err)
	}
	resp := &eveyv1.GetAlbumsResponse{}
	for _, a := range albums {
		resp.Albums = append(resp.Albums, &eveyv1.MusicAlbum{
			Id:          a.ID,
			Title:       a.Title,
			Artist:      a.Artist,
			Year:        int32(a.Year),
			CoverArtUrl: a.CoverArtURL,
			TrackCount:  int32(a.TrackCount),
			Duration:    a.Duration,
		})
	}
	return resp, nil
}

func (h *musicHandler) GetArtists(ctx context.Context, req *eveyv1.GetArtistsRequest) (*eveyv1.GetArtistsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.music.GetArtists")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("musicHandler.GetArtists: %w", err)
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	artists, err := h.svc.GetArtists(ctx, shared.LibraryID(req.LibraryId))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get artists", err)
	}
	resp := &eveyv1.GetArtistsResponse{}
	for _, a := range artists {
		resp.Artists = append(resp.Artists, &eveyv1.MusicArtist{
			Id:          a.ID,
			Name:        a.Name,
			TrackCount:  int32(a.TrackCount),
			AlbumCount:  int32(a.AlbumCount),
			CoverArtUrl: a.CoverArtURL,
		})
	}
	return resp, nil
}

func (h *musicHandler) SearchMusic(ctx context.Context, req *eveyv1.SearchMusicRequest) (*eveyv1.SearchMusicResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.music.SearchMusic")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("musicHandler.SearchMusic: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	} else if limit > maxPageSize {
		limit = maxPageSize
	}

	// A specific library must be access-checked up front. An empty library_id is
	// the global search bar: search everything, then drop hits from libraries
	// the caller can't access (mirrors SearchMedia's allowedIDs model). Never
	// trust the index to scope access.
	if req.LibraryId != "" {
		if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, err
		}
	}

	searchLimit := limit
	allowed, err := h.allowedLibraryIDs(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	// Cross-library + restricted user: over-fetch so post-filtering still fills
	// the page.
	if req.LibraryId == "" && allowed != nil {
		searchLimit = limit * 4
	}

	tracks, err := h.svc.Search(ctx, shared.LibraryID(req.LibraryId), req.Query, searchLimit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "search music", err)
	}

	claims := interceptors.ClaimsFromContext(ctx)
	isAdmin := claims != nil && claims.IsAdmin
	resp := &eveyv1.SearchMusicResponse{}
	for _, t := range tracks {
		if allowed != nil && !allowed[string(t.LibraryID)] {
			continue
		}
		resp.Tracks = append(resp.Tracks, trackToProto(t, isAdmin))
		if len(resp.Tracks) >= limit {
			break
		}
	}
	return resp, nil
}

func (h *musicHandler) ScanMusicDirectory(ctx context.Context, req *eveyv1.ScanMusicDirectoryRequest) (*eveyv1.ScanMusicDirectoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.music.ScanMusicDirectory")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("musicHandler.ScanMusicDirectory: %w", err)
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if req.LibraryId == "" || req.Directory == "" {
		span.SetStatus(otelcodes.Error, "library_id and directory required")
		return nil, status.Error(codes.InvalidArgument, "library_id and directory required")
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	dir := filepath.Clean(req.Directory)
	if !filepath.IsAbs(dir) || strings.Contains(dir, "..") {
		span.SetStatus(otelcodes.Error, "invalid directory")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.directory_invalid")
	}
	lib, libErr := h.libQuery.GetLibrary(ctx, req.LibraryId)
	if libErr != nil || lib == nil {
		if libErr != nil {
			span.RecordError(libErr)
		}
		span.SetStatus(otelcodes.Error, "library not found")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.library.not_found")
	}
	allowed := false
	for _, p := range lib.Paths {
		cleanP := filepath.Clean(p)
		if strings.HasPrefix(dir, cleanP+string(filepath.Separator)) || dir == cleanP {
			allowed = true
			break
		}
	}
	if !allowed {
		span.SetStatus(otelcodes.Error, "directory not within library paths")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.directory_outside")
	}
	// Detach from the request context: a music scan walks the filesystem and
	// fans tag-parsing across NumCPU workers - for a large library that runs
	// for minutes and must outlive the client's fire-and-forget RPC (the
	// request ctx dies on return, and a proxy read-timeout would otherwise 504
	// and cancel the work mid-scan). Mirrors ScanPhotoDirectory. Count = -1
	// signals "scan started" (the real count lands in the logs).
	srvCtx := h.serverCtx
	if srvCtx == nil {
		srvCtx = context.Background()
	}
	libraryID, scanDir := req.LibraryId, dir
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in music scan", slog.Any("error", r))
			}
		}()
		count, scanErr := h.svc.ScanDirectory(srvCtx, libraryID, scanDir)
		if scanErr != nil {
			slog.ErrorContext(ctx, "music scan", slog.Any("err", scanErr))
		} else {
			slog.Info("music scan complete", slog.Int("count", count), slog.String("dir", scanDir))
		}
	}()
	return &eveyv1.ScanMusicDirectoryResponse{Count: -1}, nil
}

func (h *musicHandler) DeleteTrack(ctx context.Context, req *eveyv1.DeleteTrackRequest) (*emptypb.Empty, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.music.DeleteTrack")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("musicHandler.DeleteTrack: %w", err)
	}
	// Destructive op: admin-only. A non-admin with library read-access must
	// not be able to delete tracks from the server.
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	track, err := h.svc.GetTrack(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get track", err)
	}
	if track == nil {
		span.SetStatus(otelcodes.Error, "track not found")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.music.track_not_found")
	}
	if err := h.svc.DeleteTrack(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "delete track", err)
	}
	return &emptypb.Empty{}, nil
}
