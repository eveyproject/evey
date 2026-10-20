package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/ratings"
	"github.com/eveyproject/evey/internal/domain/shared"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
)

// library_handler_mediaitems.go holds the media-item read surface: get/list/
// filter/genres plus the ListMediaItems overlay pipeline (locale, overrides,
// artwork, cast) and its cursor/sort helpers.

// loadGatedMediaItem loads an item and enforces the library-access and parental
// gates, mapping every failure to an opaque NotFound so a caller cannot tell an
// absent item from a forbidden one. Returns the item on success.
func (h *libraryHandler) loadGatedMediaItem(ctx context.Context, span trace.Span, id string) (*library.MediaItem, error) {
	item, err := h.query.GetMediaItem(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "media item not found")
		slog.WarnContext(ctx, "grpc: media item query failed", slog.Any("err", err))
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	if ok, accessErr := h.checkLibraryAccess(ctx, item.LibraryID); !ok {
		if accessErr != nil {
			span.RecordError(accessErr)
			slog.WarnContext(ctx, "check library access", slog.Any("err", accessErr))
		}
		span.SetStatus(otelcodes.Error, "library access denied")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	if h.isBlockedByParental(ctx, item) {
		span.SetStatus(otelcodes.Error, "parental restricted")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	return item, nil
}

func (h *libraryHandler) GetMediaItem(ctx context.Context, req *eveyv1.GetMediaItemRequest) (*eveyv1.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetMediaItem")
	defer span.End()
	item, err := h.loadGatedMediaItem(ctx, span, req.Id)
	if err != nil {
		return nil, err
	}
	// Overlay translated metadata first (always, including fallback locale - the
	// base title may be in another language).
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
	// Manual overrides win over the auto-translation: an admin's explicit edit
	// is deliberate intent and must take precedence over a fetched localized
	// title (also the only way to correct a wrong-match title that a stale
	// translation row would otherwise keep showing).
	if h.metadataSvc != nil {
		if overrides, oErr := h.metadataSvc.ListOverrides(ctx, req.Id); oErr == nil {
			applyOverridesToItem(item, overrides)
		}
	}
	pb := mediaItemToProto(item, ctxIsAdmin(ctx))
	fillAncestorTitles(ctx, h.overlaidItemsFetch, []*eveyv1.MediaItem{pb})
	// Artwork/cast/streams are best-effort enrichments: a failure on one must
	// not fail the whole detail response. But the errors were being swallowed
	// silently, which hides real problems (e.g. streams empty in the DB or a
	// failing query looks identical to "no data"). Log them so they're
	// diagnosable instead of invisible.
	if h.metadataSvc != nil {
		if artworks, err := h.metadataSvc.GetArtwork(ctx, req.Id); err != nil {
			slog.WarnContext(ctx, "media detail: load artwork failed", slog.String("media_item_id", req.Id), slog.Any("err", err))
		} else {
			for _, a := range artworks {
				pb.Artwork = append(pb.Artwork, artworkRecordToProto(a))
			}
		}
	}
	if cast, err := h.query.GetCast(ctx, req.Id); err != nil {
		slog.WarnContext(ctx, "media detail: load cast failed", slog.String("media_item_id", req.Id), slog.Any("err", err))
	} else {
		for _, cm := range cast {
			pb.People = append(pb.People, castMemberToProto(cm))
		}
	}
	if streams, err := h.query.GetMediaStreams(ctx, req.Id); err != nil {
		slog.WarnContext(ctx, "media detail: load streams failed", slog.String("media_item_id", req.Id), slog.Any("err", err))
	} else {
		for _, s := range streams {
			pb.Streams = append(pb.Streams, mediaStreamToProto(s))
		}
	}
	return pb, nil
}

func (h *libraryHandler) GetSeriesAggregate(ctx context.Context, req *eveyv1.GetSeriesAggregateRequest) (*eveyv1.SeriesAggregate, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetSeriesAggregate")
	defer span.End()
	if _, err := h.loadGatedMediaItem(ctx, span, req.Id); err != nil {
		return nil, err
	}
	agg, err := h.query.GetSeriesAggregate(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "aggregate series", err)
	}
	return &eveyv1.SeriesAggregate{
		SeriesId:        req.Id,
		TotalDurationMs: agg.TotalDuration.Milliseconds(),
		EpisodeCount:    int32(agg.EpisodeCount),
		SeasonCount:     int32(agg.SeasonCount),
	}, nil
}

// mediaItemsFilterFromRequest builds the base query filter from the request's
// scalar criteria (sort, library, type, genre, year, parent, person). Access
// scoping and pagination are applied separately since they need ctx/span.
func mediaItemsFilterFromRequest(req *eveyv1.ListMediaItemsRequest) library.ListMediaItemsFilter {
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
		// A parent's children are a season's episodes or a series' seasons:
		// unless the client asked for another order, serve them by number. By
		// title, a season longer than one page came back as the first 500
		// titles, which no client-side re-sort can turn into episodes 1-500.
		if filter.SortField == "" {
			filter.SortField = "index_number"
		}
	}
	if req.PersonId != "" {
		filter.PersonID = &req.PersonId
	}
	return filter
}

// applyMediaItemsLibraryAccess enforces the IDOR gate when a single library is
// requested (checkLibraryAccess), or otherwise narrows filter.LibraryIDs to
// the caller's allowed libraries. Returns false when the request must be
// denied (single-library access refused, or the allowed set is unknowable);
// the caller must short-circuit in that case.
func (h *libraryHandler) applyMediaItemsLibraryAccess(ctx context.Context, req *eveyv1.ListMediaItemsRequest, filter *library.ListMediaItemsFilter) bool {
	if req.LibraryId != "" {
		if ok, accessErr := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); !ok {
			if accessErr != nil {
				slog.WarnContext(ctx, "check library access", slog.Any("err", accessErr))
			}
			return false
		}
		return true
	}
	ids, restricted, ok := h.allowedLibraryFilter(ctx)
	if !ok {
		return false
	}
	if restricted {
		filter.LibraryIDs = toLibraryIDs(ids)
	}
	return true
}

// applyMediaItemsPagination clamps page size/offset into filter and, when a
// cursor is present, decodes it and resets Offset (cursor paging takes
// precedence over offset paging). Returns a formatted gRPC error on an
// invalid cursor.
func (h *libraryHandler) applyMediaItemsPagination(ctx context.Context, span trace.Span, pagination *eveyv1.PaginationRequest, filter *library.ListMediaItemsFilter) error {
	if pagination == nil {
		return nil
	}
	filter.Limit, filter.Offset = clampPagination(pagination.PageSize, pagination.Page, 50)
	if pagination.Cursor != "" {
		cursor, err := shared.DecodeCursor(pagination.Cursor)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "invalid cursor")
			slog.WarnContext(ctx, "grpc ListMediaItems: invalid cursor", slog.Any("err", err))
			return h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.cursor_invalid")
		}
		filter.Cursor = cursor
		filter.Offset = 0
	}
	return nil
}

// overlayMediaItemsLocale overlays locale translations (title/overview/tagline)
// onto items in place, so the grid matches the language the detail page
// already uses. Best-effort: base values stay when no translation exists; a
// fetch error is logged, not fatal.
// overlaidItemsFetch is GetMediaItemsByIDs with the viewer's locale and the
// admin overrides applied, so the series/season titles stamped on an episode
// read in the same language as the episode title itself. Still two batched
// lookups per ancestor level, no per-item query.
func (h *libraryHandler) overlaidItemsFetch(ctx context.Context, ids []string) ([]*library.MediaItem, error) {
	items, err := h.query.GetMediaItemsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	h.overlayMediaItemsLocale(ctx, ids, items)
	h.overlayMediaItemsOverrides(ctx, ids, items)
	return items, nil
}

func (h *libraryHandler) overlayMediaItemsLocale(ctx context.Context, ids []string, items []*library.MediaItem) {
	overlayItemsLocale(ctx, h.query, ids, items)
}

// overlayMediaItemsOverrides applies manual metadata overrides in place. These
// win over the auto-translation too (same precedence as the detail view) so a
// corrected title shows in the grid/season list.
func (h *libraryHandler) overlayMediaItemsOverrides(ctx context.Context, ids []string, items []*library.MediaItem) {
	if h.metadataSvc == nil {
		return
	}
	overlayItemsOverrides(ctx, h.metadataSvc, ids, items)
}

// fetchMediaItemsArtwork batch-loads artwork for ids, best-effort (a fetch
// error is logged and yields a nil map, matching the original inline code).
func (h *libraryHandler) fetchMediaItemsArtwork(ctx context.Context, ids []string) map[string][]*domainmetadata.ArtworkRecord {
	if h.metadataSvc == nil {
		return nil
	}
	artworkMap, artErr := h.metadataSvc.BatchGetArtwork(ctx, ids)
	if artErr != nil {
		slog.WarnContext(ctx, "batch get artwork", slog.Any("err", artErr))
	}
	return artworkMap
}

// fetchMediaItemsCast batch-loads cast info when filtering by person, so the
// frontend can show roles; best-effort like fetchMediaItemsArtwork.
func (h *libraryHandler) fetchMediaItemsCast(ctx context.Context, req *eveyv1.ListMediaItemsRequest, ids []string) map[string][]library.CastMember {
	if req.PersonId == "" || len(ids) == 0 {
		return nil
	}
	castMap, castErr := h.query.GetCastForItems(ctx, ids)
	if castErr != nil {
		slog.WarnContext(ctx, "batch get cast for person filter", slog.Any("err", castErr))
	}
	return castMap
}

// fetchMediaItemsStreams batch-loads the streams for one page of items:
// grids and filter facets on every client read codecs/resolution off LIST
// rows, and an empty streams array is indistinguishable from "no streams" -
// which is how the quality badges and facet filters shipped dead. Best-effort
// like fetchMediaItemsArtwork.
func (h *libraryHandler) fetchMediaItemsStreams(ctx context.Context, ids []string) map[string][]*library.MediaStream {
	if len(ids) == 0 {
		return nil
	}
	streamsMap, err := h.query.GetStreamsForItems(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "batch get streams for listing", slog.Any("err", err))
		return nil
	}
	return streamsMap
}

// mediaItemsToProto maps items to wire format, dropping items still blocked
// by a per-tag parental restriction (the strict in-memory second pass) and
// attaching artwork/cast/streams from the batch-loaded maps.
func mediaItemsToProto(ctx context.Context, items []*library.MediaItem, pf parentalFilter, artworkMap map[string][]*domainmetadata.ArtworkRecord, castMap map[string][]library.CastMember, streamsMap map[string][]*library.MediaStream) []*eveyv1.MediaItem {
	out := make([]*eveyv1.MediaItem, 0, len(items))
	for _, item := range items {
		if len(pf.blockedTags) > 0 && ratings.HasBlockedTag(itemTags(item), pf.blockedTags) {
			continue
		}
		pb := mediaItemToProto(item, ctxIsAdmin(ctx))
		for _, a := range artworkMap[string(item.ID)] {
			pb.Artwork = append(pb.Artwork, artworkRecordToProto(a))
		}
		for _, cm := range castMap[string(item.ID)] {
			pb.People = append(pb.People, castMemberToProto(cm))
		}
		for _, s := range streamsMap[string(item.ID)] {
			pb.Streams = append(pb.Streams, mediaStreamToProto(s))
		}
		out = append(out, pb)
	}
	return out
}

func (h *libraryHandler) ListMediaItems(ctx context.Context, req *eveyv1.ListMediaItemsRequest) (*eveyv1.ListMediaItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.ListMediaItems")
	defer span.End()
	filter := mediaItemsFilterFromRequest(req)
	if !h.applyMediaItemsLibraryAccess(ctx, req, &filter) {
		return &eveyv1.ListMediaItemsResponse{Pagination: &eveyv1.PaginationResponse{}}, nil
	}
	if err := h.applyMediaItemsPagination(ctx, span, req.Pagination, &filter); err != nil {
		return nil, err
	}
	pf := h.resolveParentalFilter(ctx)
	filter.AllowedRatings = pf.allowedRatings

	total, err := h.query.CountMediaItems(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "count media items", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, shared.ErrCursorMismatch) {
			span.SetStatus(otelcodes.Error, "cursor mismatch")
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.cursor_invalid")
		}
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list media items", err)
	}
	// SQL pre-filter is permissive on unknown / empty official_rating and
	// can't see foreign certifications_by_country, so it lets through items
	// like Outbreak (cert AU/GB only, empty official_rating) on a kids
	// profile. Apply the strict in-memory predicate as a second pass to
	// match the feeds / playback gates.
	// The cursor is read from the page the database returned, before that pass:
	// a page one short after it is not the last page, and a cursor built from
	// it ended a kids profile's walk at the first title it could not see.
	nextCursor := nextMediaItemsCursor(items, filter)
	items = filterOutParentalBlocked(pf, items)
	resp := &eveyv1.ListMediaItemsResponse{
		Pagination: &eveyv1.PaginationResponse{
			TotalItems: int32(total),
			Page:       req.Pagination.GetPage(),
			PageSize:   req.Pagination.GetPageSize(),
			NextCursor: nextCursor,
		},
	}
	if ps := int(req.Pagination.GetPageSize()); ps > 0 {
		resp.Pagination.TotalPages = int32((total + ps - 1) / ps)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.ID))
	}
	h.overlayMediaItemsLocale(ctx, ids, items)
	h.overlayMediaItemsOverrides(ctx, ids, items)
	artworkMap := h.fetchMediaItemsArtwork(ctx, ids)
	castMap := h.fetchMediaItemsCast(ctx, req, ids)
	streamsMap := h.fetchMediaItemsStreams(ctx, ids)
	resp.Items = mediaItemsToProto(ctx, items, pf, artworkMap, castMap, streamsMap)
	fillAncestorTitles(ctx, h.overlaidItemsFetch, resp.Items)
	return resp, nil
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
		// Mirror the SQL comparison column exactly. Falling back to
		// m.Title here would make the cursor's SortValue diverge from
		// what the SQL keyset clause sees in `mi.sort_title`, breaking
		// pagination on every row where sort_title is empty (NULL-ish
		// rows from older scans before scan-title was populated).
		return m.SortTitle
	case "added_at", "date_added":
		return m.SortAddedAt().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
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
	case "index_number":
		if m.IndexNumber != nil {
			return fmt.Sprintf("%010d", *m.IndexNumber)
		}
		return ""
	case "rating", "community_rating":
		// BOTH spellings. The API emits "rating" (mediaSortFieldFromProto) while
		// the SQL column map accepts either - and this switch used to know only
		// "community_rating", so every rating cursor fell through to the default
		// and encoded the item's UUID as its sort value. Page 2 then bound that
		// UUID against a numeric column: HTTP 500, "échec du chargement" the
		// moment a viewer scrolled a library sorted by rating (prod, 2026-09-05).
		if m.Classification != nil && m.Classification.CommunityRating != nil {
			return fmt.Sprintf("%020.6f", *m.Classification.CommunityRating)
		}
		return ""
	default:
		// ⚠️ Reached only by a sort key nothing above knows, which cannot
		// paginate: the id is not comparable against the sort column.
		// contractguard/sort_field_contract_test.go fails the build rather than
		// let a new sort field arrive here.
		return string(m.ID)
	}
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
				slog.WarnContext(ctx, "check library access", slog.Any("err", accessErr))
			}
			return &eveyv1.FilterMediaItemsResponse{}, nil
		}
		libID := shared.LibraryID(req.LibraryId)
		filter.LibraryID = &libID
	} else {
		ids, restricted, ok := h.allowedLibraryFilter(ctx)
		if !ok {
			return &eveyv1.FilterMediaItemsResponse{}, nil
		}
		if restricted {
			filter.LibraryIDs = toLibraryIDs(ids)
		}
	}
	if req.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_UNSPECIFIED {
		t := mediaItemTypeFromProto[req.Type]
		filter.Type = &t
	}
	// Multi-select chips win over the single free-text genre. Both feed
	// filter.Genres, which the repo OR-matches server-side.
	if len(req.Genres) > 0 {
		filter.Genres = req.Genres
	} else if req.Genre != "" {
		filter.Genres = []string{req.Genre}
	}

	// Parental gate: mirror ListMediaItems so the filter screen can't surface
	// adult content to a kids profile. SQL pre-filter on official_rating plus
	// the strict in-memory second pass (foreign certifications the SQL can't
	// see). Without this, FilterMediaItems leaks restricted items.
	pf := h.resolveParentalFilter(ctx)
	filter.AllowedRatings = pf.allowedRatings

	total, err := h.query.CountMediaItems(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "count", err)
	}
	items, err := h.query.ListMediaItems(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list", err)
	}
	// Paged on the rows the database returned, before the parental pass: a
	// page it shortened is not the last one, and the next page starts after
	// every row read, seen or not.
	nextOffset := filter.Offset + len(items)
	items = filterOutParentalBlocked(pf, items)
	resp := &eveyv1.FilterMediaItemsResponse{
		Total:      int32(total),
		NextOffset: int32(nextOffset),
		// An empty read never claims more, whatever the count says: a client
		// would ask for the same offset forever.
		HasMore: nextOffset > filter.Offset && nextOffset < total,
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.ID))
	}
	// Same hydration as ListMediaItems, through the same shared mapper: the
	// filter panel only narrows the set, it must not change what a card is.
	// Building the proto by hand here dropped the operator's title override,
	// the locale overlay and the artwork placeholder the moment any filter was
	// on - silently, since a card with a stale title looks perfectly fine.
	// Cast stays nil: it is only ever loaded for the person filter, which this
	// RPC does not have.
	h.overlayMediaItemsLocale(ctx, ids, items)
	h.overlayMediaItemsOverrides(ctx, ids, items)
	artworkMap := h.fetchMediaItemsArtwork(ctx, ids)
	streamsMap := h.fetchMediaItemsStreams(ctx, ids)
	resp.Items = mediaItemsToProto(ctx, items, pf, artworkMap, nil, streamsMap)
	fillAncestorTitles(ctx, h.overlaidItemsFetch, resp.Items)
	return resp, nil
}

func (h *libraryHandler) ListGenres(ctx context.Context, req *eveyv1.ListGenresRequest) (*eveyv1.ListGenresResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.ListGenres")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if req.LibraryId == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.id_required")
	}
	if ok, accessErr := h.checkLibraryAccess(ctx, shared.LibraryID(req.LibraryId)); !ok {
		if accessErr != nil {
			slog.WarnContext(ctx, "check library access", slog.Any("err", accessErr))
		}
		return &eveyv1.ListGenresResponse{}, nil
	}
	genres, err := h.query.ListGenres(ctx, shared.LibraryID(req.LibraryId))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list genres", err)
	}
	return &eveyv1.ListGenresResponse{Genres: genres}, nil
}

// GetLibraryFilterFacets answers the filter panel's codec, resolution and
// language chips from the whole library, where the clients used to read them
// off a sample of loaded items and never offered what the sample missed.
//
// Same gates as ListMediaItems for the same library_id: checkLibraryAccess
// (an IDOR answers an empty set, like ListGenres), then the parental filter,
// applied whole inside the facet query (library.ParentalRule). A facet value
// carried only by a hidden title therefore never reaches a kids profile. It
// used to walk the library 500 items at a time in Go to find the visible ones.
func (h *libraryHandler) GetLibraryFilterFacets(ctx context.Context, req *eveyv1.GetLibraryFilterFacetsRequest) (*eveyv1.GetLibraryFilterFacetsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetLibraryFilterFacets")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if req.LibraryId == "" {
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.id_required")
	}
	libID := shared.LibraryID(req.LibraryId)
	if ok, accessErr := h.checkLibraryAccess(ctx, libID); !ok {
		if accessErr != nil {
			slog.WarnContext(ctx, "check library access", slog.Any("err", accessErr))
		}
		return &eveyv1.GetLibraryFilterFacetsResponse{}, nil
	}
	filter := library.ListMediaItemsFilter{LibraryID: &libID}
	if req.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_UNSPECIFIED {
		t := mediaItemTypeFromProto[req.Type]
		filter.Type = &t
	}
	pf := h.resolveParentalFilter(ctx)
	if pf.blockAll {
		// The unreadable profile sees no item at all, so it has nothing to
		// filter on either; no need to ask the database.
		return &eveyv1.GetLibraryFilterFacetsResponse{}, nil
	}
	if pf.enforced() {
		filter.Parental = pf.rule()
	}
	facets, err := h.query.ListStreamFacets(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list stream facets", err)
	}
	resp := &eveyv1.GetLibraryFilterFacetsResponse{
		VideoCodecs:       facets.VideoCodecs,
		AudioLanguages:    facets.AudioLanguages,
		SubtitleLanguages: facets.SubtitleLanguages,
	}
	for _, tier := range facets.ResolutionTiers() {
		resp.ResolutionTiers = append(resp.ResolutionTiers, int32(tier))
	}
	return resp, nil
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

// --- Public (non-sensitive) media projection ---
//
// GetPublicMediaItem/ListPublicMediaItems return only id/title/type/year/
// overview/poster - never file paths, sizes, provider ids or streams. They
// honour the same library-access and parental gates as the full handlers, so
// the `public` permission cannot widen what the caller may see, only narrow the
// fields. This is the read surface a plugin may be granted.
