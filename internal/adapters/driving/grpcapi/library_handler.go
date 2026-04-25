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
	"github.com/evey-project/evey/internal/pkg/region"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
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
	ctx, span := tracing.StartSpan(ctx, "grpc.library.CreateLibrary")
	defer span.End()
	lib, err := library.NewLibrary(
		shared.LibraryID(id.New("lib-")),
		req.Name,
		library.LibraryType(libraryTypeFromProto[req.Type]),
		library.WithLibraryPaths(req.Paths),
		library.WithLibraryScanInterval(360),
		library.WithLibraryScanOnStartup(req.ScanOnStartup),
		library.WithLibraryRealtimeMonitoring(req.RealtimeMonitoring),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.cmd.CreateLibrary(ctx, lib); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("create library", err)
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) GetLibrary(ctx context.Context, req *eveyv1.GetLibraryRequest) (*eveyv1.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetLibrary")
	defer span.End()
	lib, err := h.query.GetLibrary(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "library not found")
		return nil, status.Errorf(codes.NotFound, "library not found: %v", err)
	}
	if ok, accessErr := h.checkLibraryAccess(ctx, lib.ID); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.Warn("check library access", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, status.Errorf(codes.NotFound, "library not found")
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) ListLibraries(ctx context.Context, _ *eveyv1.ListLibrariesRequest) (*eveyv1.ListLibrariesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.ListLibraries")
	defer span.End()
	libs, err := h.query.ListLibraries(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("list libraries", err)
	}
	libs, err = h.filterLibrariesByAccess(ctx, libs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("filter libraries", err)
	}
	resp := &eveyv1.ListLibrariesResponse{}
	for _, lib := range libs {
		resp.Libraries = append(resp.Libraries, libraryToProto(lib))
	}
	return resp, nil
}

func (h *libraryHandler) UpdateLibrary(ctx context.Context, req *eveyv1.UpdateLibraryRequest) (*eveyv1.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.UpdateLibrary")
	defer span.End()
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	lib, err := h.query.GetLibrary(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "library not found")
		return nil, status.Errorf(codes.NotFound, "library not found: %v", err)
	}
	// Defense-in-depth IDOR check: RBAC already restricts this RPC to admins,
	// but we re-check here so a future RBAC misconfiguration does not silently
	// expose write access to other libraries.
	if ok, accessErr := h.checkLibraryAccess(ctx, lib.ID); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.Warn("update library access check", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, status.Errorf(codes.NotFound, "library not found")
	}
	if req.Name != nil {
		lib.Name = *req.Name
	}
	if len(req.Paths) > 0 {
		for _, p := range req.Paths {
			if !filepath.IsAbs(p) || !strings.HasPrefix(filepath.Clean(p), "/media") {
				span.SetStatus(otelcodes.Error, "invalid path")
				return nil, status.Errorf(codes.InvalidArgument, "invalid path: must be an absolute path under /media")
			}
		}
		lib.Paths = req.Paths
	}
	if err := h.cmd.UpdateLibrary(ctx, lib); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("update library", err)
	}
	return libraryToProto(lib), nil
}

func (h *libraryHandler) DeleteLibrary(ctx context.Context, req *eveyv1.DeleteLibraryRequest) (*eveyv1.DeleteLibraryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.DeleteLibrary")
	defer span.End()
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	// Defense-in-depth IDOR check: RBAC already restricts this RPC to admins,
	// but explicitly re-verify access for non-admin claims so a future RBAC
	// misconfiguration cannot silently expose deletion of other libraries.
	// Admins (and contexts without userAccess wired) short-circuit.
	if ok, accessErr := h.checkLibraryAccess(ctx, shared.LibraryID(req.Id)); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.Warn("delete library access check", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, status.Errorf(codes.NotFound, "library not found")
	}
	if err := h.cmd.DeleteLibrary(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("delete library", err)
	}
	return &eveyv1.DeleteLibraryResponse{}, nil
}

func (h *libraryHandler) ScanLibrary(req *eveyv1.ScanLibraryRequest, stream eveyv1.LibraryService_ScanLibraryServer) error {
	ctx, span := tracing.StartSpan(stream.Context(), "grpc.library.ScanLibrary")
	defer span.End()
	added, updated, err := h.cmd.ScanLibrary(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, applibrary.ErrScanInProgress) {
			span.SetStatus(otelcodes.Error, "scan in progress")
			return h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.library.scan_in_progress")
		}
		span.SetStatus(otelcodes.Error, err.Error())
		return internalError("scan library", err)
	}
	if err := stream.Send(&eveyv1.ScanProgress{
		LibraryId:      req.Id,
		Phase:          eveyv1.ScanPhase_SCAN_PHASE_COMPLETE,
		Percent:        100,
		TotalItems:     int32(added + updated),
		ProcessedItems: int32(added + updated),
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	return nil
}

func (h *libraryHandler) GetMediaItem(ctx context.Context, req *eveyv1.GetMediaItemRequest) (*eveyv1.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetMediaItem")
	defer span.End()
	item, err := h.query.GetMediaItem(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "media item not found")
		return nil, status.Errorf(codes.NotFound, "media item not found: %v", err)
	}
	if ok, accessErr := h.checkLibraryAccess(ctx, item.LibraryID); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.Warn("check library access", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	if blocked, parentalErr := h.isBlockedByParental(ctx, item); blocked {
		if parentalErr != nil {
			span.RecordError(parentalErr)
			slog.Warn("parental check", slog.Any("err", parentalErr))
		}
		span.SetStatus(otelcodes.Error, "parental restricted")
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

func (h *libraryHandler) GetSeriesAggregate(ctx context.Context, req *eveyv1.GetSeriesAggregateRequest) (*eveyv1.SeriesAggregate, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetSeriesAggregate")
	defer span.End()
	item, err := h.query.GetMediaItem(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "media item not found")
		return nil, status.Errorf(codes.NotFound, "media item not found: %v", err)
	}
	if ok, accessErr := h.checkLibraryAccess(ctx, item.LibraryID); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.Warn("check library access", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	if blocked, parentalErr := h.isBlockedByParental(ctx, item); blocked {
		if parentalErr != nil {
			span.RecordError(parentalErr)
			slog.Warn("parental check", slog.Any("err", parentalErr))
		}
		span.SetStatus(otelcodes.Error, "parental restricted")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.library.parental_restricted")
	}
	agg, err := h.query.GetSeriesAggregate(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "aggregate series: %v", err)
	}
	return &eveyv1.SeriesAggregate{
		SeriesId:        req.Id,
		TotalDurationMs: agg.TotalDurationMs,
		EpisodeCount:    int32(agg.EpisodeCount),
		SeasonCount:     int32(agg.SeasonCount),
	}, nil
}

func (h *libraryHandler) ListMediaItems(ctx context.Context, req *eveyv1.ListMediaItemsRequest) (*eveyv1.ListMediaItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.ListMediaItems")
	defer span.End()
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
		if req.Pagination.Cursor != "" {
			cursor, err := shared.DecodeCursor(req.Pagination.Cursor)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, "invalid cursor")
				return nil, status.Errorf(codes.InvalidArgument, "invalid cursor: %v", err)
			}
			filter.Cursor = cursor
			filter.Offset = 0
		}
	}
	pf := h.resolveParentalFilter(ctx)
	filter.AllowedRatings = pf.allowedRatings

	total, err := h.query.CountMediaItems(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("count media items", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, shared.ErrCursorMismatch) {
			span.SetStatus(otelcodes.Error, "cursor mismatch")
			return nil, status.Error(codes.InvalidArgument, "cursor was generated for a different sort field")
		}
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("list media items", err)
	}
	// SQL pre-filter is permissive on unknown / empty official_rating and
	// can't see foreign certifications_by_country, so it lets through items
	// like Outbreak (cert AU/GB only, empty official_rating) on a kids
	// profile. Apply the strict in-memory predicate as a second pass to
	// match the feeds / playback gates.
	if pf.maxAge >= 0 || len(pf.blockedTags) > 0 {
		filtered := items[:0]
		for _, it := range items {
			if mediaItemBlockedByParental(pf, it) {
				continue
			}
			filtered = append(filtered, it)
		}
		items = filtered
	}
	resp := &eveyv1.ListMediaItemsResponse{
		Pagination: &eveyv1.PaginationResponse{
			TotalItems: int32(total),
			Page:       req.Pagination.GetPage(),
			PageSize:   req.Pagination.GetPageSize(),
			NextCursor: nextMediaItemsCursor(items, filter),
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
		if len(pf.blockedTags) > 0 && ratings.HasBlockedTag(itemTags(item), pf.blockedTags) {
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
	ctx, span := tracing.StartSpan(ctx, "grpc.library.SearchMedia")
	defer span.End()
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
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
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
		if pf.maxAge >= 0 && !ratings.IsAllowedByAge(pf.region, hit.OfficialRating, pf.maxAge) {
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
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetPerson")
	defer span.End()
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	person, err := h.query.GetPerson(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("get person", err)
	}
	if person == nil {
		span.SetStatus(otelcodes.Error, "person not found")
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
	allowedRatings []string // SQL pre-filter: all ratings (across countries) allowed under maxAge
	maxAge         int      // -1 = no age filter
	region         string   // profile's resolved region for per-country cert lookup
	blockedTags    []string
	locale         string   // profile locale, used to resolve region when not set
}

func (h *libraryHandler) resolveParentalFilter(ctx context.Context) parentalFilter {
	return resolveParentalFilterFromUserQuery(ctx, h.userQuery)
}

// resolveParentalFilterFromUserQuery is the package-level entry point used
// by every handler that needs to filter results by the active profile's
// parental settings. Returns a "no-op" filter (maxAge=-1, no blocked tags)
// when the caller is unauthenticated, the profile lookup fails, or the
// profile has no effective enforcement (i.e. is not a kids profile).
func resolveParentalFilterFromUserQuery(ctx context.Context, userQuery *appuser.QueryService) parentalFilter {
	if userQuery == nil {
		return parentalFilter{maxAge: -1}
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.ProfileID == "" {
		return parentalFilter{maxAge: -1}
	}
	profile, err := userQuery.GetProfileByID(ctx, claims.ProfileID)
	if err != nil {
		return parentalFilter{maxAge: -1}
	}
	ps := profile.EffectiveParental()
	if ps == nil {
		return parentalFilter{maxAge: -1}
	}
	age := -1
	if ps.MaxAge != nil {
		age = *ps.MaxAge
	}
	return parentalFilter{
		allowedRatings: ratings.AllowedRatingsForAge(age),
		maxAge:         age,
		region:         region.Resolve(profile.Region, "", profile.Locale),
		locale:         profile.Locale,
		blockedTags:    ps.BlockedTags,
	}
}

// mediaItemBlockedByParental returns true if the given item violates the
// resolved parental filter. Conservative for kids: when no rating is
// resolvable (no certifications anywhere, no official rating mapped),
// the item is blocked rather than silently allowed — otherwise a movie
// like Outbreak with only "AU: MA 15+" / "GB: 15" certifications would
// slip past a profile region'd to FR because there is no FR cert and
// the global ratings.IsAllowedByAge defaults to permissive.
//
// We pick the worst-case (highest) age cap across:
//   1. the profile's region cert (if present)
//   2. the OfficialRating field
//   3. every other country cert we can map
//
// If none of those map to a known min-age, we fail closed for the kids
// profile.
func mediaItemBlockedByParental(pf parentalFilter, item *library.MediaItem) bool {
	if item == nil {
		return false
	}
	if pf.maxAge < 0 && len(pf.blockedTags) == 0 {
		return false
	}
	if pf.maxAge >= 0 {
		worstAge := -1
		var certByCountry map[string]string
		var officialRating string
		if item.Classification != nil {
			certByCountry = item.Classification.CertificationsByCountry
			officialRating = item.Classification.OfficialRating
		}
		if pf.region != "" && certByCountry != nil {
			if r, ok := certByCountry[pf.region]; ok && r != "" {
				if age := ratings.MinAgeFor(pf.region, r); age > worstAge {
					worstAge = age
				}
			}
		}
		if officialRating != "" {
			if age := ratings.MinAgeFor(pf.region, officialRating); age > worstAge {
				worstAge = age
			}
		}
		// Worst-case across every known foreign cert. A kid profile in FR
		// shouldn't be the place to discover that an MA 15+ Australian rating
		// existed all along.
		for country, rating := range certByCountry {
			if rating == "" {
				continue
			}
			if age := ratings.MinAgeFor(country, rating); age > worstAge {
				worstAge = age
			}
		}
		if worstAge < 0 {
			// No rating resolvable. Fail closed for kids — admin can edit the
			// item's metadata override to expose it explicitly if it really is
			// child-appropriate.
			return true
		}
		if worstAge > pf.maxAge {
			return true
		}
	}
	if ratings.HasBlockedTag(itemTags(item), pf.blockedTags) {
		return true
	}
	return false
}

func (h *libraryHandler) isBlockedByParental(ctx context.Context, item *library.MediaItem) (bool, error) {
	pf := h.resolveParentalFilter(ctx)
	return mediaItemBlockedByParental(pf, item), nil
}

func itemTags(item *library.MediaItem) []string {
	if item == nil || item.Catalog == nil {
		return nil
	}
	return item.Catalog.Tags
}

// nextMediaItemsCursor encodes a keyset cursor pointing past the last
// returned item. Empty when the page is short (final page) or when the
// caller did not request cursor pagination.
func nextMediaItemsCursor(items []*library.MediaItem, filter library.ListMediaItemsFilter) string {
	if len(items) == 0 {
		return ""
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if len(items) < limit {
		return ""
	}
	last := items[len(items)-1]
	sortKey := filter.SortField
	if sortKey == "" {
		sortKey = "title"
	}
	return shared.EncodeCursor(shared.Cursor{
		SortField: sortKey,
		SortValue: mediaItemSortValue(last, sortKey),
		LastID:    string(last.ID),
	})
}

func mediaItemSortValue(m *library.MediaItem, sortKey string) string {
	switch sortKey {
	case "title", "sort_title":
		if m.SortTitle != "" {
			return m.SortTitle
		}
		return m.Title
	case "added_at", "date_added":
		return m.AddedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	case "year":
		if m.Catalog != nil && m.Catalog.Year != nil {
			return fmt.Sprintf("%010d", *m.Catalog.Year)
		}
		return ""
	case "premiere_date":
		if m.Catalog != nil && m.Catalog.PremiereDate != nil {
			return m.Catalog.PremiereDate.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		}
		return ""
	case "community_rating":
		if m.Classification != nil && m.Classification.CommunityRating != nil {
			return fmt.Sprintf("%020.6f", *m.Classification.CommunityRating)
		}
		return ""
	default:
		return string(m.ID)
	}
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
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetLibrarySchedule")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if h.scheduler == nil {
		return &eveyv1.GetLibraryScheduleResponse{}, nil
	}
	sched, err := h.scheduler.GetSchedule(ctx, req.Id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &eveyv1.GetLibraryScheduleResponse{}, nil
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("get library schedule", err)
	}
	return &eveyv1.GetLibraryScheduleResponse{
		Schedule: scheduleToProto(sched),
	}, nil
}

func (h *libraryHandler) SetLibrarySchedule(ctx context.Context, req *eveyv1.SetLibraryScheduleRequest) (*eveyv1.LibrarySchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.SetLibrarySchedule")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if h.scheduler == nil {
		span.SetStatus(otelcodes.Error, "scheduler not available")
		return nil, status.Error(codes.Unimplemented, "scheduler not available")
	}
	if req.Cron == "" {
		span.SetStatus(otelcodes.Error, "cron expression required")
		return nil, status.Error(codes.InvalidArgument, "cron expression required")
	}
	sched, err := h.scheduler.SetSchedule(ctx, req.Id, req.Cron, req.Enabled)
	if err != nil {
		span.RecordError(err)
		if strings.HasPrefix(err.Error(), "invalid cron") {
			span.SetStatus(otelcodes.Error, "invalid cron expression")
			return nil, status.Errorf(codes.InvalidArgument, "invalid cron expression: %v", err)
		}
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("set schedule", err)
	}
	return scheduleToProto(sched), nil
}

func (h *libraryHandler) DeleteLibrarySchedule(ctx context.Context, req *eveyv1.DeleteLibraryScheduleRequest) (*eveyv1.DeleteLibraryScheduleResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.DeleteLibrarySchedule")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if !claims.IsAdmin {
		span.SetStatus(otelcodes.Error, "admin only")
		return nil, status.Error(codes.PermissionDenied, "admin only")
	}
	if h.scheduler != nil {
		if delErr := h.scheduler.DeleteSchedule(ctx, req.Id); delErr != nil {
			span.RecordError(delErr)
			slog.Warn("delete schedule", slog.Any("err", delErr))
		}
	}
	return &eveyv1.DeleteLibraryScheduleResponse{}, nil
}

func (h *libraryHandler) FilterMediaItems(ctx context.Context, req *eveyv1.FilterMediaItemsRequest) (*eveyv1.FilterMediaItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.FilterMediaItems")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
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
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("count", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
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
				if item.Catalog == nil {
					item.Catalog = &library.MediaCatalog{}
				}
				item.Catalog.Year = &y
			}
		case "genres":
			parts := strings.Split(v, ",")
			genres := make([]string, 0, len(parts))
			for _, p := range parts {
				if g := strings.TrimSpace(p); g != "" {
					genres = append(genres, g)
				}
			}
			if item.Catalog == nil {
				item.Catalog = &library.MediaCatalog{}
			}
			item.Catalog.Genres = genres
		case "community_rating":
			var r float64
			if _, err := fmt.Sscanf(v, "%f", &r); err == nil {
				if item.Classification == nil {
					item.Classification = &library.MediaClassification{}
				}
				item.Classification.CommunityRating = &r
			}
		}
	}
}

