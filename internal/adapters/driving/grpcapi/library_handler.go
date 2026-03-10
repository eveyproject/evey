package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type libraryHandler struct {
	eveyv1.UnimplementedLibraryServiceServer
	cmd           *applibrary.CommandService
	query         *applibrary.QueryService
	metadataSvc   *appmetadata.Service
	libraryAccess user.LibraryAccessRepository
}

func (h *libraryHandler) CreateLibrary(ctx context.Context, req *eveyv1.CreateLibraryRequest) (*eveyv1.Library, error) {
	lib := &library.Library{
		ID:                  id.New("lib-"),
		Name:                req.Name,
		Type:                libraryTypeFromProto[req.Type],
		Paths:               req.Paths,
		Enabled:             true,
		ScanOnStartup:       req.ScanOnStartup,
		RealtimeMonitoring:  req.RealtimeMonitoring,
		ScanIntervalMinutes: 360,
	}
	if err := h.cmd.CreateLibrary(ctx, lib); err != nil {
		return nil, status.Errorf(codes.Internal, "create library: %v", err)
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) GetLibrary(ctx context.Context, req *eveyv1.GetLibraryRequest) (*eveyv1.Library, error) {
	lib, err := h.query.GetLibrary(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "library not found: %v", err)
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) ListLibraries(ctx context.Context, _ *eveyv1.ListLibrariesRequest) (*eveyv1.ListLibrariesResponse, error) {
	libs, err := h.query.ListLibraries(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list libraries: %v", err)
	}
	libs, err = h.filterLibrariesByAccess(ctx, libs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "filter libraries: %v", err)
	}
	resp := &eveyv1.ListLibrariesResponse{}
	for _, lib := range libs {
		resp.Libraries = append(resp.Libraries, libraryToProto(lib))
	}
	return resp, nil
}

func (h *libraryHandler) DeleteLibrary(ctx context.Context, req *eveyv1.DeleteLibraryRequest) (*eveyv1.DeleteLibraryResponse, error) {
	if err := h.cmd.DeleteLibrary(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete library: %v", err)
	}
	return &eveyv1.DeleteLibraryResponse{}, nil
}

func (h *libraryHandler) ScanLibrary(req *eveyv1.ScanLibraryRequest, stream eveyv1.LibraryService_ScanLibraryServer) error {
	added, updated, err := h.cmd.ScanLibrary(stream.Context(), req.Id)
	if err != nil {
		return status.Errorf(codes.Internal, "scan library: %v", err)
	}
	return stream.Send(&eveyv1.ScanProgress{
		LibraryId:      req.Id,
		Phase:          eveyv1.ScanPhase_SCAN_PHASE_COMPLETE,
		Percent:        100,
		TotalItems:     int32(added + updated),
		ProcessedItems: int32(added + updated),
	})
}

func (h *libraryHandler) GetMediaItem(ctx context.Context, req *eveyv1.GetMediaItemRequest) (*eveyv1.MediaItem, error) {
	item, err := h.query.GetMediaItem(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "media item not found: %v", err)
	}
	if ok, _ := h.checkLibraryAccess(ctx, item.LibraryID); !ok {
		return nil, status.Errorf(codes.NotFound, "media item not found")
	}
	pb := mediaItemToProto(item)
	if h.metadataSvc != nil {
		if artworks, err := h.metadataSvc.GetArtwork(ctx, req.Id); err == nil {
			for _, a := range artworks {
				pb.Artwork = append(pb.Artwork, artworkRecordToProto(a))
			}
		}
	}
	if cast, err := h.query.GetCast(ctx, req.Id); err == nil {
		for _, cm := range cast {
			pb.People = append(pb.People, castMemberToProto(cm))
		}
	}
	return pb, nil
}

func (h *libraryHandler) ListMediaItems(ctx context.Context, req *eveyv1.ListMediaItemsRequest) (*eveyv1.ListMediaItemsResponse, error) {
	filter := library.ListMediaItemsFilter{
		SortField: mediaSortFieldFromProto[req.SortField],
		SortOrder: sortOrderFromProto[req.SortOrder],
	}
	if req.LibraryId != "" {
		filter.LibraryID = &req.LibraryId
	}
	if req.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_UNSPECIFIED {
		t := mediaItemTypeFromProto[req.Type]
		filter.Type = &t
	}
	if req.Genre != "" {
		filter.Genres = []string{req.Genre}
	}
	if req.Year != 0 {
		y := int(req.Year)
		filter.Year = &y
	}
	if req.ParentId != "" {
		filter.ParentID = &req.ParentId
	}
	if req.LibraryId != "" {
		if ok, _ := h.checkLibraryAccess(ctx, req.LibraryId); !ok {
			return &eveyv1.ListMediaItemsResponse{Pagination: &eveyv1.PaginationResponse{}}, nil
		}
	} else {
		if ids, err := h.getAllowedLibraryIDs(ctx); err == nil && ids != nil {
			filter.LibraryIDs = ids
		}
	}
	if req.Pagination != nil {
		filter.Limit = int(req.Pagination.PageSize)
		filter.Offset = int((req.Pagination.Page - 1) * req.Pagination.PageSize)
	}

	total, err := h.query.CountMediaItems(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "count media items: %v", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list media items: %v", err)
	}
	resp := &eveyv1.ListMediaItemsResponse{
		Pagination: &eveyv1.PaginationResponse{
			TotalItems: int32(total),
			Page:       req.Pagination.GetPage(),
			PageSize:   req.Pagination.GetPageSize(),
		},
	}
	for _, item := range items {
		resp.Items = append(resp.Items, mediaItemToProto(item))
	}
	return resp, nil
}

func (h *libraryHandler) SearchMedia(ctx context.Context, req *eveyv1.SearchMediaRequest) (*eveyv1.SearchMediaResponse, error) {
	filters := make(map[string]string)
	if req.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_UNSPECIFIED {
		filters["type"] = mediaItemTypeFromProto[req.Type]
	}
	limit := int64(req.Limit)
	if limit <= 0 {
		limit = 40
	}
	hits, total, err := h.query.SearchMedia(ctx, req.Query, filters, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search media: %v", err)
	}
	resp := &eveyv1.SearchMediaResponse{
		TotalHits: int32(total),
	}
	for _, hit := range hits {
		mi := &eveyv1.MediaItem{
			Id:    hit.ID,
			Title: hit.Title,
			Type:  mediaItemTypeToProto[hit.Type],
		}
		if hit.Year != nil {
			mi.Year = int32(*hit.Year)
		}
		resp.Items = append(resp.Items, mi)
	}
	return resp, nil
}

func (h *libraryHandler) filterLibrariesByAccess(ctx context.Context, libs []*library.Library) ([]*library.Library, error) {
	allowedIDs, err := h.getAllowedLibraryIDs(ctx)
	if err != nil {
		return nil, err
	}
	if allowedIDs == nil {
		return libs, nil
	}
	allowed := make(map[string]bool, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	filtered := make([]*library.Library, 0, len(libs))
	for _, lib := range libs {
		if allowed[lib.ID] {
			filtered = append(filtered, lib)
		}
	}
	return filtered, nil
}

func (h *libraryHandler) getAllowedLibraryIDs(ctx context.Context) ([]string, error) {
	if h.libraryAccess == nil {
		return nil, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	ids, err := h.libraryAccess.GetByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

func (h *libraryHandler) checkLibraryAccess(ctx context.Context, libraryID string) (bool, error) {
	if h.libraryAccess == nil {
		return true, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return true, nil
	}
	return h.libraryAccess.HasAccess(ctx, claims.UserID, libraryID)
}
