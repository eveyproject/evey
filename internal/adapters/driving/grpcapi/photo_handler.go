package grpcapi

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type photoHandler struct {
	eveyv1.UnimplementedPhotoServiceServer
	svc           *appphoto.Service
	libraryAccess user.LibraryAccessRepository
	libQuery      *applibrary.QueryService
	serverCtx     context.Context
}

func (h *photoHandler) checkLibraryAccess(ctx context.Context, libraryID string) error {
	if h.libraryAccess == nil {
		return nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil
	}
	ok, err := h.libraryAccess.HasAccess(ctx, claims.UserID, libraryID)
	if err != nil {
		return status.Errorf(codes.Internal, "check library access: %v", err)
	}
	if !ok {
		return status.Error(codes.PermissionDenied, "no access to library")
	}
	return nil
}

func (h *photoHandler) ListPhotos(ctx context.Context, req *eveyv1.ListPhotosRequest) (*eveyv1.ListPhotosResponse, error) {
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("photoHandler.ListPhotos: %w", err)
	}
	if req.LibraryId == "" {
		return nil, status.Error(codes.InvalidArgument, "library_id required")
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := int(req.Offset)
	photos, total, err := h.svc.ListPhotos(ctx, req.LibraryId, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list photos: %v", err)
	}
	resp := &eveyv1.ListPhotosResponse{Total: int32(total)}
	for _, p := range photos {
		resp.Photos = append(resp.Photos, photoToProto(p))
	}
	return resp, nil
}

func (h *photoHandler) GetPhoto(ctx context.Context, req *eveyv1.GetPhotoRequest) (*eveyv1.Photo, error) {
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("photoHandler.GetPhoto: %w", err)
	}
	p, err := h.svc.GetPhoto(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get photo: %v", err)
	}
	if p == nil {
		return nil, status.Error(codes.NotFound, "photo not found")
	}
	if err := h.checkLibraryAccess(ctx, p.LibraryID); err != nil {
		return nil, err
	}
	return photoToProto(*p), nil
}

func (h *photoHandler) ListPhotoAlbums(ctx context.Context, req *eveyv1.ListPhotoAlbumsRequest) (*eveyv1.ListPhotoAlbumsResponse, error) {
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("photoHandler.ListPhotoAlbums: %w", err)
	}
	if req.LibraryId == "" {
		return nil, status.Error(codes.InvalidArgument, "library_id required")
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	albums, err := h.svc.GetAlbums(ctx, req.LibraryId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list photo albums: %v", err)
	}
	resp := &eveyv1.ListPhotoAlbumsResponse{}
	for _, a := range albums {
		resp.Albums = append(resp.Albums, &eveyv1.PhotoAlbum{
			Name:         a.Name,
			LibraryId:    a.LibraryID,
			PhotoCount:   int32(a.PhotoCount),
			CoverPhotoId: a.CoverPhotoID,
		})
	}
	return resp, nil
}

func (h *photoHandler) SearchPhotos(ctx context.Context, req *eveyv1.SearchPhotosRequest) (*eveyv1.SearchPhotosResponse, error) {
	if _, err := requireClaims(ctx); err != nil {
		return nil, fmt.Errorf("photoHandler.SearchPhotos: %w", err)
	}
	if req.LibraryId == "" || req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "library_id and query required")
	}
	if err := h.checkLibraryAccess(ctx, req.LibraryId); err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	photos, err := h.svc.Search(ctx, req.LibraryId, req.Query, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search photos: %v", err)
	}
	resp := &eveyv1.SearchPhotosResponse{}
	for _, p := range photos {
		resp.Photos = append(resp.Photos, photoToProto(p))
	}
	return resp, nil
}

func (h *photoHandler) ScanPhotoDirectory(ctx context.Context, req *eveyv1.ScanPhotoDirectoryRequest) (*eveyv1.ScanPhotoDirectoryResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("photoHandler.ScanPhotoDirectory: %w", err)
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
	srvCtx := h.serverCtx
	if srvCtx == nil {
		srvCtx = ctx
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in photo scan", slog.Any("error", r))
			}
		}()
		n, scanErr := h.svc.ScanDirectory(srvCtx, req.LibraryId, dir)
		if scanErr != nil {
			slog.Error("photo scan", slog.Any("err", scanErr))
		} else {
			slog.Info("photo scan complete", slog.Int("count", n), slog.String("dir", dir))
		}
	}()
	return &eveyv1.ScanPhotoDirectoryResponse{Count: -1}, nil
}

func (h *photoHandler) DeletePhoto(ctx context.Context, req *eveyv1.DeletePhotoRequest) (*emptypb.Empty, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("photoHandler.DeletePhoto: %w", err)
	}
	p, err := h.svc.GetPhoto(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get photo: %v", err)
	}
	if p == nil {
		return nil, status.Error(codes.NotFound, "photo not found")
	}
	if !claims.IsAdmin {
		if accessErr := h.checkLibraryAccess(ctx, p.LibraryID); accessErr != nil {
			return nil, accessErr
		}
	}
	if err := h.svc.DeletePhoto(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete photo: %v", err)
	}
	return &emptypb.Empty{}, nil
}
