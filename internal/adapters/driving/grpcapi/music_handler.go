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
}

func (h *musicHandler) checkLibraryAccess(ctx context.Context, libraryID string) error {
	if h.userAccess == nil {
		return nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil
	}
	ok, err := h.userAccess.HasLibraryAccess(ctx, claims.UserID, libraryID)
	if err != nil {
		return internalError("check library access", err)
	}
	if !ok {
		return status.Error(codes.PermissionDenied, "no access to library")
	}
	return nil
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
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := int(req.Offset)
	tracks, total, err := h.svc.ListTracks(ctx, shared.LibraryID(req.LibraryId), limit, offset, req.Search)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("list tracks", err)
	}
	claims, _ := requireClaims(ctx)
	resp := &eveyv1.ListTracksResponse{Total: int32(total)}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, trackToProto(t, claims != nil && claims.IsAdmin))
	}
	return resp, nil
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
		return nil, internalError("get albums", err)
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
		return nil, internalError("get artists", err)
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
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	tracks, err := h.svc.Search(ctx, shared.LibraryID(req.LibraryId), req.Query, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("search music", err)
	}
	claims, _ := requireClaims(ctx)
	resp := &eveyv1.SearchMusicResponse{}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, trackToProto(t, claims != nil && claims.IsAdmin))
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
		return nil, status.Error(codes.PermissionDenied, "admin only")
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
		return nil, status.Error(codes.InvalidArgument, "invalid directory")
	}
	lib, libErr := h.libQuery.GetLibrary(ctx, req.LibraryId)
	if libErr != nil || lib == nil {
		if libErr != nil {
			span.RecordError(libErr)
		}
		span.SetStatus(otelcodes.Error, "library not found")
		return nil, status.Error(codes.NotFound, "library not found")
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
		return nil, status.Error(codes.InvalidArgument, "directory not within library paths")
	}
	count, scanErr := h.svc.ScanDirectory(ctx, req.LibraryId, dir)
	if scanErr != nil {
		slog.Error("music scan", slog.Any("err", scanErr))
		span.RecordError(scanErr)
		span.SetStatus(otelcodes.Error, scanErr.Error())
		return nil, internalError("scan error", scanErr)
	}
	return &eveyv1.ScanMusicDirectoryResponse{Count: int32(count)}, nil
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
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	track, err := h.svc.GetTrack(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("get track", err)
	}
	if track == nil {
		span.SetStatus(otelcodes.Error, "track not found")
		return nil, status.Error(codes.NotFound, "track not found")
	}
	if err := h.svc.DeleteTrack(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("delete track", err)
	}
	return &emptypb.Empty{}, nil
}
