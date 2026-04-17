package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/ratings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type libraryHandler struct {
	eveyv1.UnimplementedLibraryServiceServer
	cmd           *applibrary.CommandService
	query         *applibrary.QueryService
	metadataSvc   *appmetadata.QueryService
	userAccess    *appuser.AccessService
	userQuery     *appuser.QueryService
	translator    *eveyi18n.Translator
	scheduler     *applibrary.Scheduler
}

func (h *libraryHandler) CreateLibrary(ctx context.Context, req *eveyv1.CreateLibraryRequest) (*eveyv1.Library, error) {
	lib := &library.Library{
		ID:                  shared.LibraryID(id.New("lib-")),
		Name:                req.Name,
		Type:                library.LibraryType(libraryTypeFromProto[req.Type]),
		Paths:               req.Paths,
		Enabled:             true,
		ScanOnStartup:       req.ScanOnStartup,
		RealtimeMonitoring:  req.RealtimeMonitoring,
		ScanIntervalMinutes: 360,
	}
	if err := h.cmd.CreateLibrary(ctx, lib); err != nil {
		return nil, internalError("create library", err)
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) GetLibrary(ctx context.Context, req *eveyv1.GetLibraryRequest) (*eveyv1.Library, error) {
	lib, err := h.query.GetLibrary(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "library not found: %v", err)
	}
	if ok, accessErr := h.checkLibraryAccess(ctx, lib.ID); !ok {
		if accessErr != nil {
			slog.Warn("check library access", slog.Any("err", accessErr))
		}
		return nil, status.Errorf(codes.NotFound, "library not found")
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) ListLibraries(ctx context.Context, _ *eveyv1.ListLibrariesRequest) (*eveyv1.ListLibrariesResponse, error) {
	libs, err := h.query.ListLibraries(ctx)
	if err != nil {
		return nil, internalError("list libraries", err)
	}
	libs, err = h.filterLibrariesByAccess(ctx, libs)
	if err != nil {
		return nil, internalError("filter libraries", err)
	}
	resp := &eveyv1.ListLibrariesResponse{}
	for _, lib := range libs {
		resp.Libraries = append(resp.Libraries, libraryToProto(lib))
	}
	return resp, nil
}

func (h *libraryHandler) UpdateLibrary(ctx context.Context, req *eveyv1.UpdateLibraryRequest) (*eveyv1.Library, error) {
	if req.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	lib, err := h.query.GetLibrary(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "library not found: %v", err)
	}
	if req.Name != nil {
		lib.Name = *req.Name
	}
	if len(req.Paths) > 0 {
		for _, p := range req.Paths {
			if !filepath.IsAbs(p) || !strings.HasPrefix(filepath.Clean(p), "/media") {
				return nil, status.Errorf(codes.InvalidArgument, "invalid path: must be an absolute path under /media")
			}
		}
		lib.Paths = req.Paths
	}
	if err := h.cmd.UpdateLibrary(ctx, lib); err != nil {
		return nil, internalError("update library", err)
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) DeleteLibrary(ctx context.Context, req *eveyv1.DeleteLibraryRequest) (*eveyv1.DeleteLibraryResponse, error) {
	if err := h.cmd.DeleteLibrary(ctx, req.Id); err != nil {
		return nil, internalError("delete library", err)
	}
	return &eveyv1.DeleteLibraryResponse{}, nil
}

func (h *libraryHandler) ScanLibrary(req *eveyv1.ScanLibraryRequest, stream eveyv1.LibraryService_ScanLibraryServer) error {
	added, updated, err := h.cmd.ScanLibrary(stream.Context(), req.Id)
	if err != nil {
		if errors.Is(err, applibrary.ErrScanInProgress) {
			return h.translator.TranslatedError(stream.Context(), codes.FailedPrecondition, "errors.library.scan_in_progress")
		}
		return internalError("scan library", err)
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
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	if blocked, parentalErr := h.isBlockedByParental(ctx, item); blocked {
		if parentalErr != nil {
			slog.Warn("parental check", slog.Any("err", parentalErr))
		}
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.library.parental_restricted")
	}
	if h.metadataSvc != nil {
		if overrides, oErr := h.metadataSvc.ListOverrides(ctx, req.Id); oErr == nil {
			applyOverridesToItem(item, overrides)
		}
	}
	// Overlay translated metadata (always, including fallback locale — base title may be in another language)
	locale := eveyi18n.LocaleFromCtx(ctx)
	if locale != "" {
		if trans, tErr := h.query.GetMediaItemTranslation(ctx, req.Id, locale); tErr == nil && trans != nil {
			if trans.Title != "" {
				item.Title = trans.Title
			}
			if trans.Overview != "" {
				item.Overview = trans.Overview
			}
			if trans.Tagline != "" {
				item.Tagline = trans.Tagline
			}
		}
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
		libID := shared.LibraryID(req.LibraryId)
		filter.LibraryID = &libID
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
	if req.PersonId != "" {
		filter.PersonID = &req.PersonId
	}
	if req.LibraryId != "" {
		if ok, accessErr := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); !ok {
			if accessErr != nil {
				slog.Warn("check library access", slog.Any("err", accessErr))
			}
			return &eveyv1.ListMediaItemsResponse{Pagination: &eveyv1.PaginationResponse{}}, nil
		}
	} else {
		if ids, err := h.getAllowedLibraryIDs(ctx); err == nil && ids != nil {
			filter.LibraryIDs = toLibraryIDs(ids)
		}
	}
	if req.Pagination != nil {
		filter.Limit, filter.Offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, 50)
	}
	pf := h.resolveParentalFilter(ctx)
	filter.AllowedRatings = pf.allowedRatings

	total, err := h.query.CountMediaItems(ctx, filter)
	if err != nil {
		return nil, internalError("count media items", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		return nil, internalError("list media items", err)
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
		ids = append(ids, string(item.ID))
	}
	var artworkMap map[string][]*domainmetadata.ArtworkRecord
	if h.metadataSvc != nil {
		var artErr error
		artworkMap, artErr = h.metadataSvc.BatchGetArtwork(ctx, ids)
		if artErr != nil {
			slog.Warn("batch get artwork", slog.Any("err", artErr))
		}
	}

	// When filtering by person, include cast info so the frontend can show roles
	var castMap map[string][]library.CastMember
	if req.PersonId != "" && len(ids) > 0 {
		var castErr error
		castMap, castErr = h.query.GetCastForItems(ctx, ids)
		if castErr != nil {
			slog.Warn("batch get cast for person filter", slog.Any("err", castErr))
		}
	}

	for _, item := range items {
		if len(pf.blockedTags) > 0 && ratings.HasBlockedTag(item.Tags, pf.blockedTags) {
			continue
		}
		pb := mediaItemToProto(item)
		for _, a := range artworkMap[string(item.ID)] {
			pb.Artwork = append(pb.Artwork, artworkRecordToProto(a))
		}
		for _, cm := range castMap[string(item.ID)] {
			pb.People = append(pb.People, castMemberToProto(cm))
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
	if req.Genre != "" {
		filters["genres"] = req.Genre
	}
	if req.YearMin > 0 {
		filters["year_min"] = fmt.Sprintf("%d", req.YearMin)
	}
	if req.YearMax > 0 {
		filters["year_max"] = fmt.Sprintf("%d", req.YearMax)
	}
	var sort []string
	if req.SortField != eveyv1.MediaSortField_MEDIA_SORT_FIELD_UNSPECIFIED {
		dir := "asc"
		if req.SortOrder == eveyv1.SortOrder_SORT_ORDER_DESC {
			dir = "desc"
		}
		switch req.SortField {
		case eveyv1.MediaSortField_MEDIA_SORT_FIELD_TITLE:
			sort = append(sort, "title:"+dir)
		case eveyv1.MediaSortField_MEDIA_SORT_FIELD_YEAR:
			sort = append(sort, "year:"+dir)
		case eveyv1.MediaSortField_MEDIA_SORT_FIELD_DATE_ADDED:
			sort = append(sort, "added_at:"+dir)
		}
	}
	limit := int64(req.Limit)
	if limit <= 0 {
		limit = 40
	}
	hits, total, err := h.query.SearchMedia(ctx, req.Query, filters, sort, limit)
	if err != nil {
		return nil, internalError("search media", err)
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	pf := h.resolveParentalFilter(ctx)
	resp := &eveyv1.SearchMediaResponse{
		TotalHits: int32(total),
	}
	for _, hit := range hits {
		if allowedIDs != nil && !slices.Contains(allowedIDs, string(hit.LibraryID)) {
			continue
		}
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

func (h *libraryHandler) GetPerson(ctx context.Context, req *eveyv1.GetPersonRequest) (*eveyv1.GetPersonResponse, error) {
	if req.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	person, err := h.query.GetPerson(ctx, req.Id)
	if err != nil {
		return nil, internalError("get person", err)
	}
	if person == nil {
		return nil, status.Errorf(codes.NotFound, "person not found")
	}
	// Overlay translated biography if user locale != default
	personLocale := eveyi18n.LocaleFromCtx(ctx)
	if personLocale != "" && personLocale != eveyi18n.FallbackLocale {
		if trans, tErr := h.query.GetPersonTranslation(ctx, person.ID, personLocale); tErr == nil && trans != nil {
			if trans.Biography != "" {
				person.Biography = trans.Biography
			}
		}
	}
	resp := &eveyv1.GetPersonResponse{
		Id:           person.ID,
		Name:         person.Name,
		Biography:    person.Biography,
		Birthday:     person.Birthday,
		Deathday:     person.Deathday,
		PlaceOfBirth: person.PlaceOfBirth,
		KnownFor:     person.KnownFor,
	}
	if person.ThumbPath != nil && *person.ThumbPath != "" {
		resp.ThumbUrl = *person.ThumbPath
	}
	if len(person.ProviderIDs) > 0 {
		resp.ProviderIds = person.ProviderIDs
	}
	return resp, nil
}

type parentalFilter struct {
	allowedRatings []string // for SQL filter (list endpoint)
	maxRating      string   // for in-memory filter (search endpoint)
	blockedTags    []string
}

func (h *libraryHandler) resolveParentalFilter(ctx context.Context) parentalFilter {
	if h.userQuery == nil {
		return parentalFilter{}
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.ProfileID == "" {
		return parentalFilter{}
	}
	profile, err := h.userQuery.GetProfileByID(ctx, claims.ProfileID)
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
		return nil, fmt.Errorf("libraryHandler.filterLibrariesByAccess: %w", err)
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
		if allowed[string(lib.ID)] {
			filtered = append(filtered, lib)
		}
	}
	return filtered, nil
}

func (h *libraryHandler) getAllowedLibraryIDs(ctx context.Context) ([]string, error) {
	if h.userAccess == nil {
		return nil, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("libraryHandler.getAllowedLibraryIDs: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

func toLibraryIDs(ids []string) []shared.LibraryID {
	out := make([]shared.LibraryID, len(ids))
	for i, id := range ids {
		out[i] = shared.LibraryID(id)
	}
	return out
}

func (h *libraryHandler) checkLibraryAccess(ctx context.Context, libraryID shared.LibraryID) (bool, error) {
	if h.userAccess == nil {
		return true, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return true, nil
	}
	return h.userAccess.HasLibraryAccess(ctx, claims.UserID, string(libraryID))
}

func (h *libraryHandler) GetLibrarySchedule(ctx context.Context, req *eveyv1.GetLibraryScheduleRequest) (*eveyv1.GetLibraryScheduleResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if h.scheduler == nil {
		return &eveyv1.GetLibraryScheduleResponse{}, nil
	}
	sched, err := h.scheduler.GetSchedule(ctx, req.Id)
	if err != nil {
		return &eveyv1.GetLibraryScheduleResponse{}, nil
	}
	return &eveyv1.GetLibraryScheduleResponse{
		Schedule: scheduleToProto(sched),
	}, nil
}

func (h *libraryHandler) SetLibrarySchedule(ctx context.Context, req *eveyv1.SetLibraryScheduleRequest) (*eveyv1.LibrarySchedule, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if h.scheduler == nil {
		return nil, status.Error(codes.Unimplemented, "scheduler not available")
	}
	if req.Cron == "" {
		return nil, status.Error(codes.InvalidArgument, "cron expression required")
	}
	sched, err := h.scheduler.SetSchedule(ctx, req.Id, req.Cron, req.Enabled)
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid cron") {
			return nil, status.Errorf(codes.InvalidArgument, "invalid cron expression: %v", err)
		}
		return nil, internalError("set schedule", err)
	}
	return scheduleToProto(sched), nil
}

func (h *libraryHandler) DeleteLibrarySchedule(ctx context.Context, req *eveyv1.DeleteLibraryScheduleRequest) (*eveyv1.DeleteLibraryScheduleResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if h.scheduler != nil {
		if delErr := h.scheduler.DeleteSchedule(ctx, req.Id); delErr != nil {
			slog.Warn("delete schedule", slog.Any("err", delErr))
		}
	}
	return &eveyv1.DeleteLibraryScheduleResponse{}, nil
}

func (h *libraryHandler) FilterMediaItems(ctx context.Context, req *eveyv1.FilterMediaItemsRequest) (*eveyv1.FilterMediaItemsResponse, error) {
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	filter := library.ListMediaItemsFilter{
		SortField: mediaSortFieldFromProto[req.SortField],
		SortOrder: sortOrderFromProto[req.SortOrder],
		Limit:     int(req.Limit),
		Offset:    int(req.Offset),
		MinRating: float64(req.MinRating),
		YearFrom:  int(req.YearFrom),
		YearTo:    int(req.YearTo),
	}
	if req.LibraryId != "" {
		if ok, accessErr := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); !ok {
			if accessErr != nil {
				slog.Warn("check library access", slog.Any("err", accessErr))
			}
			return &eveyv1.FilterMediaItemsResponse{}, nil
		}
		libID := shared.LibraryID(req.LibraryId)
		filter.LibraryID = &libID
	} else {
		if ids, idErr := h.getAllowedLibraryIDs(ctx); idErr == nil && ids != nil {
			filter.LibraryIDs = toLibraryIDs(ids)
		}
	}
	if req.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_UNSPECIFIED {
		t := mediaItemTypeFromProto[req.Type]
		filter.Type = &t
	}
	if req.Genre != "" {
		filter.Genres = []string{req.Genre}
	}

	total, err := h.query.CountMediaItems(ctx, filter)
	if err != nil {
		return nil, internalError("count", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		return nil, internalError("list", err)
	}
	resp := &eveyv1.FilterMediaItemsResponse{Total: int32(total)}
	for _, item := range items {
		resp.Items = append(resp.Items, mediaItemToProto(item))
	}
	return resp, nil
}

func scheduleToProto(s *library.ScanSchedule) *eveyv1.LibrarySchedule {
	pb := &eveyv1.LibrarySchedule{
		LibraryId: string(s.LibraryID),
		Cron:      s.CronExpression,
		Enabled:   s.Enabled,
	}
	if s.LastRunAt != nil {
		pb.LastRunAt = toTimestamppbPtr(s.LastRunAt)
	}
	if s.NextRunAt != nil {
		pb.NextRunAt = toTimestamppbPtr(s.NextRunAt)
	}
	return pb
}

func applyOverridesToItem(item *library.MediaItem, overrides []*domainmetadata.MetadataOverride) {
	for _, o := range overrides {
		v, _ := o.CustomValue.(string)
		if v == "" {
			continue
		}
		switch o.FieldName {
		case "title":
			item.Title = v
		case "original_title":
			item.OriginalTitle = v
		case "overview":
			item.Overview = v
		case "year":
			var y int
			if _, err := fmt.Sscanf(v, "%d", &y); err == nil {
				item.Year = &y
			}
		case "genres":
			parts := strings.Split(v, ",")
			genres := make([]string, 0, len(parts))
			for _, p := range parts {
				if g := strings.TrimSpace(p); g != "" {
					genres = append(genres, g)
				}
			}
			item.Genres = genres
		case "community_rating":
			var r float64
			if _, err := fmt.Sscanf(v, "%f", &r); err == nil {
				item.CommunityRating = &r
			}
		}
	}
}

