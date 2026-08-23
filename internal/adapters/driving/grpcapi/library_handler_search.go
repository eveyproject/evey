package grpcapi

import (
	"context"
	"fmt"
	"slices"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// library_handler_search.go holds the search RPCs: full-text media search and
// person lookup.

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
	// Scope to a single library when the caller is on a library page.
	// Meilisearch + SQL fallback both filter on library_id natively, so
	// this turns the previously cosmetic library search into a real
	// server-side query. Library access is still re-enforced below via
	// allowedIDs - passing an unauthorised library_id can't widen
	// results, only narrow them.
	if req.LibraryId != "" {
		filters["library_id"] = req.LibraryId
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
	} else if limit > maxPageSize {
		// Cap the page size like every other list endpoint (clampLimitOffset),
		// so a client cannot request an unbounded result set from the search
		// index / SQL fallback (memory + query-load DoS).
		limit = maxPageSize
	}
	hits, total, err := h.query.SearchMedia(ctx, req.Query, filters, sort, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "search media", err)
	}
	allowedIDs, allowErr := h.getAllowedLibraryIDs(ctx)
	if allowErr != nil {
		// The allowed set is unknown: keep the deny-shaped empty set returned
		// by getAllowedLibraryIDs so the loop below drops every hit, instead
		// of widening the search on a grant-store outage.
		span.RecordError(allowErr)
	}
	pf := h.resolveParentalFilter(ctx)
	resp := &eveyv1.SearchMediaResponse{
		TotalHits: int32(total),
	}
	// The search index does not carry per-country certifications (it only has a
	// single, often-empty OfficialRating), so the indexed fields are not a sound
	// basis for a kids filter. When a parental filter is active, resolve the full
	// items and run the same fail-closed, worst-case-across-countries check used
	// everywhere else. Non-kids profiles (and admins) skip the fetch entirely.
	var parentalBlocked map[string]bool
	if pf.enforced() {
		ids := make([]string, 0, len(hits))
		for _, hit := range hits {
			ids = append(ids, hit.ID)
		}
		parentalBlocked = make(map[string]bool, len(hits))
		items, ferr := h.query.GetMediaItemsByIDs(ctx, ids)
		if ferr != nil {
			// Cannot resolve classification for a kids profile → fail closed:
			// every hit is treated as blocked rather than leaked.
			span.RecordError(ferr)
			for _, hit := range hits {
				parentalBlocked[hit.ID] = true
			}
		} else {
			byID := make(map[string]*library.MediaItem, len(items))
			for _, it := range items {
				byID[string(it.ID)] = it
			}
			for _, hit := range hits {
				it := byID[hit.ID]
				if it == nil || mediaItemBlockedByParental(pf, it) {
					parentalBlocked[hit.ID] = true
				}
			}
		}
	}
	dropped := 0
	for _, hit := range hits {
		if allowedIDs != nil && !slices.Contains(allowedIDs, string(hit.LibraryID)) {
			dropped++
			continue
		}
		if parentalBlocked != nil && parentalBlocked[hit.ID] {
			dropped++
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
	// Search hits are minimal index rows: the document carries no parent link
	// and no index number, so a hit inside a show comes back naked. Resolve
	// EPISODE and SEASON hits once against the DB and stamp both fields back.
	//
	// The season number is not cosmetic. A season has no name of its own: the
	// scan freezes an English "Season N" on the row, and every client derives
	// the shown label from indexNumber instead (seasonLabel, displayTitle.ts).
	// With no number to derive from, a season found through search fell back
	// to that frozen title - "Season 2" in the middle of a French page.
	// Hydrating here needs no reindex and no new index field.
	inShowByID := make(map[string]*eveyv1.MediaItem)
	for _, mi := range resp.Items {
		if mi.Type == eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE ||
			mi.Type == eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SEASON {
			inShowByID[mi.Id] = mi
		}
	}
	if len(inShowByID) > 0 {
		ids := make([]string, 0, len(inShowByID))
		for id := range inShowByID {
			ids = append(ids, id)
		}
		for id, full := range fetchItemMap(ctx, h.query.GetMediaItemsByIDs, ids) {
			inShowByID[id].ParentId = full.ParentID
			if full.IndexNumber != nil {
				inShowByID[id].IndexNumber = int32(*full.IndexNumber)
			}
		}
		fillAncestorTitles(ctx, h.query.GetMediaItemsByIDs, resp.Items)
	}
	// TotalHits is the backend's unfiltered tally, so on its own it announces
	// matches in libraries the caller cannot see (or that parental blocks):
	// the item list came back empty while the count said "3 results". That is
	// the existence oracle the stream routes avoid by answering 404, not 403.
	// Only correct it when something was actually dropped - otherwise keep the
	// backend total, which legitimately exceeds the returned page.
	if dropped > 0 {
		resp.TotalHits = int32(len(resp.Items))
	}
	return resp, nil
}

func (h *libraryHandler) GetPerson(ctx context.Context, req *eveyv1.GetPersonRequest) (*eveyv1.GetPersonResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetPerson")
	defer span.End()
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	person, err := h.query.GetPerson(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get person", err)
	}
	if person == nil {
		span.SetStatus(otelcodes.Error, "person not found")
		return nil, status.Error(codes.NotFound, "person not found")
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
