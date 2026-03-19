package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/library"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/user"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
	"github.com/gravity-zero/evey/internal/pkg/id"
	"github.com/gravity-zero/evey/internal/pkg/ratings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type libraryHandler struct {
	eveyv1.UnimplementedLibraryServiceServer
	cmd           *applibrary.CommandService
	query         *applibrary.QueryService
	metadataSvc   *appmetadata.QueryService
	libraryAccess user.LibraryAccessRepository
	profiles      user.ProfileRepository
	translator    *eveyi18n.Translator
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
	if ok, accessErr := h.checkLibraryAccess(ctx, item.LibraryID); !ok {
		if accessErr != nil {
			slog.Warn("check library access", slog.Any("err", accessErr))
		}
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "media.not_found")
	}
	if blocked, parentalErr := h.isBlockedByParental(ctx, item); blocked {
		if parentalErr != nil {
			slog.Warn("parental check", slog.Any("err", parentalErr))
		}
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "library.parental_restricted")
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
		if ok, accessErr := h.checkLibraryAccess(ctx, req.LibraryId); !ok {
			if accessErr != nil {
				slog.Warn("check library access", slog.Any("err", accessErr))
			}
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
	pf := h.resolveParentalFilter(ctx)
	filter.AllowedRatings = pf.allowedRatings

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
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	var artworkMap map[string][]*domainmetadata.ArtworkRecord
	if h.metadataSvc != nil {
		var artErr error
		artworkMap, artErr = h.metadataSvc.BatchGetArtwork(ctx, ids)
		if artErr != nil {
			slog.Warn("batch get artwork", slog.Any("err", artErr))
		}
	}

	for _, item := range items {
		if len(pf.blockedTags) > 0 && ratings.HasBlockedTag(item.Tags, pf.blockedTags) {
			continue
		}
		pb := mediaItemToProto(item)
		for _, a := range artworkMap[item.ID] {
			pb.Artwork = append(pb.Artwork, artworkRecordToProto(a))
		}
		resp.Items = append(resp.Items, pb)
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
	pf := h.resolveParentalFilter(ctx)
	resp := &eveyv1.SearchMediaResponse{
		TotalHits: int32(total),
	}
	for _, hit := range hits {
		if pf.maxRating != "" && !ratings.IsAllowed(hit.OfficialRating, pf.maxRating) {
			continue
		}
		if len(pf.blockedTags) > 0 && ratings.HasBlockedTag(hit.Tags, pf.blockedTags) {
			continue
		}
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

type parentalFilter struct {
	allowedRatings []string // for SQL filter (list endpoint)
	maxRating      string   // for in-memory filter (search endpoint)
	blockedTags    []string
}

func (h *libraryHandler) resolveParentalFilter(ctx context.Context) parentalFilter {
	if h.profiles == nil {
		return parentalFilter{}
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.ProfileID == "" {
		return parentalFilter{}
	}
	profile, err := h.profiles.GetByID(ctx, claims.ProfileID)
	if err != nil || profile.Parental == nil || !profile.Parental.Enabled {
		return parentalFilter{}
	}
	ps := profile.Parental
	return parentalFilter{
		allowedRatings: ratings.AllowedRatingsFor(ps.MaxRating),
		maxRating:      ps.MaxRating,
		blockedTags:    ps.BlockedTags,
	}
}

func (h *libraryHandler) isBlockedByParental(ctx context.Context, item *library.MediaItem) (bool, error) {
	pf := h.resolveParentalFilter(ctx)
	if pf.maxRating == "" && len(pf.blockedTags) == 0 {
		return false, nil
	}
	if pf.maxRating != "" && !ratings.IsAllowed(item.OfficialRating, pf.maxRating) {
		return true, nil
	}
	if ratings.HasBlockedTag(item.Tags, pf.blockedTags) {
		return true, nil
	}
	return false, nil
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
	if ids == nil {
		ids = []string{}
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

