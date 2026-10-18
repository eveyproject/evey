package grpcapi

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appphoto "github.com/eveyproject/evey/internal/app/photo"
	appuser "github.com/eveyproject/evey/internal/app/user"
	photodomain "github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/auth"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type photoHandler struct {
	eveyv1.UnimplementedPhotoServiceServer
	svc        *appphoto.Service
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
	serverCtx  context.Context
	translator *eveyi18n.Translator
}

func (h *photoHandler) checkLibraryAccess(ctx context.Context, libraryID shared.LibraryID) error {
	return libraryAccessOrStatus(ctx, h.translator, h.userAccess, string(libraryID), "photoHandler.checkLibraryAccess")
}

func (h *photoHandler) ListPhotos(ctx context.Context, req *eveyv1.ListPhotosRequest) (*eveyv1.ListPhotosResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.photo.ListPhotos")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("photoHandler.ListPhotos: %w", err)
	}
	if req.LibraryId == "" {
		span.SetStatus(otelcodes.Error, "library_id required")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.id_required")
	}
	if err := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	limit, offset := clampLimitOffset(req.Limit, req.Offset, 100)

	// Keyset path: the client already holds a cursor from a previous page, so
	// offset/total are skipped entirely - recomputing total here would pay
	// back exactly the COUNT(*) OVER() cost the cursor path exists to avoid.
	if req.Cursor != "" {
		cursor, cErr := decodePhotoCursor(req.Cursor)
		if cErr != nil {
			span.RecordError(cErr)
			span.SetStatus(otelcodes.Error, "invalid cursor")
			slog.WarnContext(ctx, "grpc ListPhotos: invalid cursor", slog.Any("err", cErr))
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.cursor_invalid")
		}
		var photos []photodomain.Photo
		var err error
		if req.Album != "" {
			photos, err = h.svc.QueryService().ListPhotosByAlbumCursor(ctx, shared.LibraryID(req.LibraryId), req.Album, cursor, limit)
		} else {
			photos, err = h.svc.QueryService().ListPhotosCursor(ctx, shared.LibraryID(req.LibraryId), cursor, limit)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "list photos", err)
		}
		resp := &eveyv1.ListPhotosResponse{}
		for _, p := range photos {
			resp.Photos = append(resp.Photos, photoToProto(p, ctxIsAdmin(ctx)))
		}
		if len(photos) >= limit {
			resp.NextCursor = encodePhotoCursor(photos[len(photos)-1])
		}
		return resp, nil
	}

	var photos []photodomain.Photo
	var total int
	var err error
	if req.Album != "" {
		photos, total, err = h.svc.ListPhotosByAlbum(ctx, shared.LibraryID(req.LibraryId), req.Album, limit, offset)
	} else {
		photos, total, err = h.svc.ListPhotos(ctx, shared.LibraryID(req.LibraryId), limit, offset)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list photos", err)
	}
	resp := &eveyv1.ListPhotosResponse{Total: int32(total)}
	for _, p := range photos {
		resp.Photos = append(resp.Photos, photoToProto(p, ctxIsAdmin(ctx)))
	}
	// Hand out the first cursor from the offset path too, otherwise keyset
	// paging is unreachable: a client can only send a cursor it was given, and
	// with page 1 refusing to produce one, nobody would ever leave the offset
	// path. Page 1 keeps its exact total, every page after it is served by
	// index seek. Clients that ignore the field are unaffected.
	if len(photos) >= limit {
		resp.NextCursor = encodePhotoCursor(photos[len(photos)-1])
	}
	return resp, nil
}

func (h *photoHandler) GetPhoto(ctx context.Context, req *eveyv1.GetPhotoRequest) (*eveyv1.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.photo.GetPhoto")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("photoHandler.GetPhoto: %w", err)
	}
	p, err := h.svc.GetPhoto(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get photo", err)
	}
	if p == nil {
		span.SetStatus(otelcodes.Error, "photo not found")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.photo.not_found")
	}
	if err := h.checkLibraryAccess(ctx, p.LibraryID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	return photoToProto(*p, ctxIsAdmin(ctx)), nil
}

func (h *photoHandler) ListPhotoAlbums(ctx context.Context, req *eveyv1.ListPhotoAlbumsRequest) (*eveyv1.ListPhotoAlbumsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.photo.ListPhotoAlbums")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("photoHandler.ListPhotoAlbums: %w", err)
	}
	if req.LibraryId == "" {
		span.SetStatus(otelcodes.Error, "library_id required")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.id_required")
	}
	if err := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	albums, err := h.svc.GetAlbums(ctx, shared.LibraryID(req.LibraryId))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list photo albums", err)
	}
	resp := &eveyv1.ListPhotoAlbumsResponse{}
	for _, a := range albums {
		resp.Albums = append(resp.Albums, &eveyv1.PhotoAlbum{
			Name:         a.Name,
			LibraryId:    string(a.LibraryID),
			PhotoCount:   int32(a.PhotoCount),
			CoverPhotoId: a.CoverPhotoID,
		})
	}
	return resp, nil
}

func (h *photoHandler) SearchPhotos(ctx context.Context, req *eveyv1.SearchPhotosRequest) (*eveyv1.SearchPhotosResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.photo.SearchPhotos")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("photoHandler.SearchPhotos: %w", err)
	}
	if req.LibraryId == "" || req.Query == "" {
		span.SetStatus(otelcodes.Error, "library_id and query required")
		return nil, status.Error(codes.InvalidArgument, "library_id and query required")
	}
	if err := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	photos, err := h.svc.Search(ctx, shared.LibraryID(req.LibraryId), req.Query, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "search photos", err)
	}
	resp := &eveyv1.SearchPhotosResponse{}
	for _, p := range photos {
		resp.Photos = append(resp.Photos, photoToProto(p, ctxIsAdmin(ctx)))
	}
	return resp, nil
}

func (h *photoHandler) ScanPhotoDirectory(ctx context.Context, req *eveyv1.ScanPhotoDirectoryRequest) (*eveyv1.ScanPhotoDirectoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.photo.ScanPhotoDirectory")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("photoHandler.ScanPhotoDirectory: %w", err)
	}
	// media:write like ScanLibrary: a pre-scope key with no permissions must
	// not inherit its owner's admin role here (interceptors.AdminReach).
	if !interceptors.AdminReach(ctx, auth.ScopeMediaWrite) {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	if req.LibraryId == "" || req.Directory == "" {
		span.SetStatus(otelcodes.Error, "library_id and directory required")
		return nil, status.Error(codes.InvalidArgument, "library_id and directory required")
	}
	if err := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); err != nil {
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
	// Detach from the request context: photo scan walks the filesystem and
	// must outlive the client's fire-and-forget RPC. When a serverCtx is
	// wired it follows the process lifetime; otherwise fall back to a fresh
	// context.Background() - NEVER the request ctx, which dies on return.
	srvCtx := h.serverCtx
	if srvCtx == nil {
		srvCtx = context.Background()
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in photo scan", slog.Any("error", r))
			}
		}()
		n, scanErr := h.svc.ScanDirectory(srvCtx, req.LibraryId, dir)
		if scanErr != nil {
			slog.ErrorContext(ctx, "photo scan", slog.Any("err", scanErr))
		} else {
			slog.Info("photo scan complete", slog.Int("count", n), slog.String("dir", dir))
		}
	}()
	return &eveyv1.ScanPhotoDirectoryResponse{Count: -1}, nil
}

func (h *photoHandler) DeletePhoto(ctx context.Context, req *eveyv1.DeletePhotoRequest) (*emptypb.Empty, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.photo.DeletePhoto")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("photoHandler.DeletePhoto: %w", err)
	}
	p, err := h.svc.GetPhoto(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get photo", err)
	}
	if p == nil {
		span.SetStatus(otelcodes.Error, "photo not found")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.photo.not_found")
	}
	if !claims.IsAdmin {
		if accessErr := h.checkLibraryAccess(ctx, p.LibraryID); accessErr != nil {
			span.RecordError(accessErr)
			span.SetStatus(otelcodes.Error, accessErr.Error())
			return nil, accessErr
		}
	}
	if err := h.svc.DeletePhoto(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "delete photo", err)
	}
	return &emptypb.Empty{}, nil
}
