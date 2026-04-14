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
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("musicHandler.ListTracks: %w", err)
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := int(req.Offset)
	tracks, total, err := h.svc.ListTracks(ctx, shared.LibraryID(req.LibraryId), limit, offset, req.Search)
	if err != nil {
		return nil, internalError("list tracks", err)
	}
	resp := &eveyv1.ListTracksResponse{Total: int32(total)}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, trackToProto(t))
	}
	return resp, nil
}

func (h *musicHandler) GetAlbums(ctx context.Context, req *eveyv1.GetAlbumsRequest) (*eveyv1.GetAlbumsResponse, error) {
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("musicHandler.GetAlbums: %w", err)
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	albums, err := h.svc.GetAlbums(ctx, shared.LibraryID(req.LibraryId))
	if err != nil {
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
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("musicHandler.GetArtists: %w", err)
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	artists, err := h.svc.GetArtists(ctx, shared.LibraryID(req.LibraryId))
	if err != nil {
		return nil, internalError("get artists", err)
	}
	resp := &eveyv1.GetArtistsResponse{}
	for _, a := range artists {
		resp.Artists = append(resp.Artists, &eveyv1.MusicArtist{
			Id:         a.ID,
			Name:       a.Name,
			TrackCount: int32(a.TrackCount),
			AlbumCount: int32(a.AlbumCount),
		})
	}
	return resp, nil
}

func (h *musicHandler) SearchMusic(ctx context.Context, req *eveyv1.SearchMusicRequest) (*eveyv1.SearchMusicResponse, error) {
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("musicHandler.SearchMusic: %w", err)
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	tracks, err := h.svc.Search(ctx, req.Query, limit)
	if err != nil {
		return nil, internalError("search music", err)
	}
	resp := &eveyv1.SearchMusicResponse{}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, trackToProto(t))
	}
	return resp, nil
}

func (h *musicHandler) ScanMusicDirectory(ctx context.Context, req *eveyv1.ScanMusicDirectoryRequest) (*eveyv1.ScanMusicDirectoryResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("musicHandler.ScanMusicDirectory: %w", err)
	}
	if !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if req.LibraryId == "" || req.Directory == "" {
		return nil, status.Error(codes.InvalidArgument, "library_id and directory required")
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	dir := filepath.Clean(req.Directory)
	if !filepath.IsAbs(dir) || strings.Contains(dir, "..") {
		return nil, status.Error(codes.InvalidArgument, "invalid directory")
	}
	lib, libErr := h.libQuery.GetLibrary(ctx, req.LibraryId)
	if libErr != nil || lib == nil {
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
		return nil, status.Error(codes.InvalidArgument, "directory not within library paths")
	}
	count, scanErr := h.svc.ScanDirectory(ctx, req.LibraryId, dir)
	if scanErr != nil {
		slog.Error("music scan", slog.Any("err", scanErr))
		return nil, internalError("scan error", scanErr)
	}
	return &eveyv1.ScanMusicDirectoryResponse{Count: int32(count)}, nil
}

func (h *musicHandler) DeleteTrack(ctx context.Context, req *eveyv1.DeleteTrackRequest) (*emptypb.Empty, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("musicHandler.DeleteTrack: %w", err)
	}
	track, err := h.svc.GetTrack(ctx, req.Id)
	if err != nil {
		return nil, internalError("get track", err)
	}
	if track == nil {
		return nil, status.Error(codes.NotFound, "track not found")
	}
	if !claims.IsAdmin {
		if accessErr := h.checkLibraryAccess(ctx, track.LibraryID); accessErr != nil {
			return nil, accessErr
		}
	}
	if err := h.svc.DeleteTrack(ctx, req.Id); err != nil {
		return nil, internalError("delete track", err)
	}
	return &emptypb.Empty{}, nil
}
