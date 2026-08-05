package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type collectionHandler struct {
	eveyv1.UnimplementedCollectionServiceServer
	svc        *appcollection.Service
	userAccess *appuser.AccessService
	libQuery   *applibrary.QueryService
	userQuery  *appuser.QueryService
}

// collectionVisibleUnderParental returns true when at least one item in the
// collection passes the active profile's parental filter. A collection that
// contains only blocked items is hidden from the listing entirely so kids
// don't see grayed-out cards leaking adult titles.
func (h *collectionHandler) collectionVisibleUnderParental(ctx context.Context, items []*library.MediaItem) bool {
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	if pf.maxAge < 0 && len(pf.blockedTags) == 0 {
		return true
	}
	for _, it := range items {
		if !mediaItemBlockedByParental(pf, it) {
			return true
		}
	}
	return false
}

// defaultMinCollectionItems is what a collection must hold to be worth
// listing. A saga you own one title of is a title: on the production server
// 230 of 236 auto-created TMDB sagas were one-title stubs, burying the six
// real ones (measured 2026-08-04). Callers that curate pass min_items=1.
const defaultMinCollectionItems = 2

func (h *collectionHandler) ListCollections(ctx context.Context, req *eveyv1.ListCollectionsRequest) (*eveyv1.ListCollectionsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.ListCollections")
	defer span.End()
	cols, err := h.svc.List(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list collections", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		span.RecordError(filterErr)
		span.SetStatus(otelcodes.Error, filterErr.Error())
		return nil, internalError(ctx, "check access", filterErr)
	}

	// Batch-fetch items for all collections in one query instead of one per collection.
	colIDs := make([]string, len(cols))
	for i, c := range cols {
		colIDs[i] = c.ID
	}
	allItems, batchErr := h.svc.GetItemsForCollections(ctx, colIDs)
	if batchErr != nil {
		span.RecordError(batchErr)
		span.SetStatus(otelcodes.Error, batchErr.Error())
		return nil, internalError(ctx, "batch collection items", batchErr)
	}

	minItems := int(req.GetMinItems())
	if minItems <= 0 {
		minItems = defaultMinCollectionItems
	}

	resp := &eveyv1.ListCollectionsResponse{}
	for _, c := range cols {
		items, skip := filterItemsByAccess(allItems[c.ID], allowed)
		if skip {
			continue
		}
		if !h.collectionVisibleUnderParental(ctx, items) {
			continue
		}
		// The threshold only applies to AUTO-created sagas. A hand-built or
		// rule-driven collection is listed even while empty: its owner made it
		// deliberately, and hiding it would leave nothing to add titles to.
		if collectionKind(c) == "saga" && len(items) < minItems {
			continue
		}
		pb := collectionToProto(c)
		// The count is what THIS caller can see: counting the filtered-out
		// members would advertise titles they cannot reach and turn the
		// number into an existence oracle for a denied library.
		pb.ItemCount = int32(len(items))
		resp.Collections = append(resp.Collections, pb)
	}
	return resp, nil
}

// filterItemsByAccess filters a collection's items to only those whose library
// is in allowedLibIDs. Returns skip=true when no items survive the filter
// (collection should be omitted from the listing).
// When allowedLibIDs is nil (admin or no access-service wired), all items are
// returned without filtering and skip is always false - matching the original
// per-collection semantics.
func filterItemsByAccess(items []*library.MediaItem, allowedLibIDs []string) ([]*library.MediaItem, bool) {
	if allowedLibIDs == nil {
		// Admin / unauthenticated path: no library-access filter.
		return items, false
	}
	var visible []*library.MediaItem
	for _, it := range items {
		if slices.Contains(allowedLibIDs, string(it.LibraryID)) {
			visible = append(visible, it)
		}
	}
	if len(visible) == 0 {
		return nil, true
	}
	return visible, false
}

func (h *collectionHandler) GetCollection(ctx context.Context, req *eveyv1.GetCollectionRequest) (*eveyv1.GetCollectionResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.GetCollection")
	defer span.End()
	col, err := h.svc.GetByID(ctx, req.Id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "collection not found")
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		span.RecordError(filterErr)
		span.SetStatus(otelcodes.Error, filterErr.Error())
		return nil, internalError(ctx, "check access", filterErr)
	}
	items, _, itemsErr := h.svc.GetItems(ctx, col.ID, library.MaxCollectionItems, 0)
	if itemsErr != nil {
		span.RecordError(itemsErr)
		span.SetStatus(otelcodes.Error, itemsErr.Error())
		return nil, internalError(ctx, "get collection items", itemsErr)
	}
	visibleItems, skip := filterItemsByAccess(items, allowed)
	if skip {
		span.SetStatus(otelcodes.Error, "collection access denied")
		return nil, status.Errorf(codes.NotFound, "collection not found")
	}
	if !h.collectionVisibleUnderParental(ctx, visibleItems) {
		span.SetStatus(otelcodes.Error, "collection blocked by parental filter")
		return nil, status.Errorf(codes.NotFound, "collection not found")
	}
	// Per-item parental filtering, exactly as GetCollectionItems does it:
	// handing the members back here without it would reopen the enumeration
	// that guard closes.
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	if pf.maxAge >= 0 || len(pf.blockedTags) > 0 {
		kept := visibleItems[:0:0]
		for _, it := range visibleItems {
			if !mediaItemBlockedByParental(pf, it) {
				kept = append(kept, it)
			}
		}
		visibleItems = kept
	}
	pb := collectionToProto(col)
	// Same access-filtered count as the listing: a detail screen that
	// announced more titles than it can show would contradict itself.
	pb.ItemCount = int32(len(visibleItems))
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultCollectionPageSize
	}
	offset := max(int(req.GetOffset()), 0)
	page := visibleItems
	if offset >= len(page) {
		page = nil
	} else {
		page = page[offset:]
	}
	if len(page) > limit {
		page = page[:limit]
	}
	return &eveyv1.GetCollectionResponse{
		Collection: pb,
		Items:      itemsToProto(page),
		Total:      int32(len(visibleItems)),
	}, nil
}

func (h *collectionHandler) DeleteCollection(ctx context.Context, req *eveyv1.DeleteCollectionRequest) (*eveyv1.DeleteCollectionResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.DeleteCollection")
	defer span.End()
	if req.GetId() == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	// Defense-in-depth IDOR check: RBAC already restricts this RPC to admins,
	// but verify caller has access to at least one library backing the
	// collection before deleting. Admins short-circuit.
	if err := h.checkCollectionAccess(ctx, req.GetId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	if err := h.svc.Delete(ctx, req.Id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "delete collection", err)
	}
	return &eveyv1.DeleteCollectionResponse{}, nil
}

func (h *collectionHandler) CreateCollection(ctx context.Context, req *eveyv1.CreateCollectionRequest) (*eveyv1.CreateCollectionResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.CreateCollection")
	defer span.End()
	if req.GetName() == "" {
		span.SetStatus(otelcodes.Error, "name is required")
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	rules := protoRulesToDomain(req.GetRules())
	var (
		created *library.Collection
		err     error
	)
	if req.GetSmart() {
		created, err = h.svc.CreateSmart(ctx, req.GetName(), rules, req.GetSortBy())
	} else {
		created, err = h.svc.Create(ctx, req.GetName(), req.GetOverview(), rules, req.GetSortBy())
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "create collection", err)
	}
	return &eveyv1.CreateCollectionResponse{Collection: collectionToProto(created)}, nil
}

func (h *collectionHandler) UpdateCollection(ctx context.Context, req *eveyv1.UpdateCollectionRequest) (*eveyv1.UpdateCollectionResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.UpdateCollection")
	defer span.End()
	if req.GetId() == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	if req.GetName() == "" {
		span.SetStatus(otelcodes.Error, "name is required")
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	// Defense-in-depth IDOR check: RBAC already restricts this RPC to admins,
	// but verify caller has access to at least one library backing the
	// collection before mutating. Admins short-circuit.
	if err := h.checkCollectionAccess(ctx, req.GetId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	rules := protoRulesToDomain(req.GetRules())
	if req.GetSmart() {
		col, err := h.svc.UpdateSmart(ctx, req.GetId(), req.GetName(), rules, req.GetSortBy())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, internalError(ctx, "update smart collection", err)
		}
		return &eveyv1.UpdateCollectionResponse{Collection: collectionToProto(col)}, nil
	}
	if err := h.svc.Update(ctx, req.GetId(), req.GetName(), req.Overview, rules, req.GetSortBy()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "update collection", err)
	}
	col, err := h.svc.GetByID(ctx, req.GetId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "fetch updated collection", err)
	}
	return &eveyv1.UpdateCollectionResponse{Collection: collectionToProto(col)}, nil
}

func (h *collectionHandler) AddCollectionItem(ctx context.Context, req *eveyv1.AddCollectionItemRequest) (*eveyv1.AddCollectionItemResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.AddCollectionItem")
	defer span.End()
	if req.GetId() == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	if req.GetMediaItemId() == "" {
		span.SetStatus(otelcodes.Error, "media_item_id is required")
		return nil, status.Errorf(codes.InvalidArgument, "media_item_id is required")
	}
	if err := h.svc.LinkMediaItem(ctx, req.GetId(), req.GetMediaItemId(), int(req.GetSortOrder())); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		// Naming something that does not exist is the caller's mistake, not the
		// server's: answering 500 blamed the wrong side and buried the reason.
		if errors.Is(err, shared.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "%v", err)
		}
		return nil, internalError(ctx, "add collection item", err)
	}
	return &eveyv1.AddCollectionItemResponse{}, nil
}

// ListCollectionsForMediaItem answers which collections hold a title, so a
// media page can say "part of the Alien saga" and link to the rest of it.
// Access-filtered like every other read here: naming a collection the caller
// cannot open would turn this into an oracle for a denied library.
func (h *collectionHandler) ListCollectionsForMediaItem(ctx context.Context, req *eveyv1.ListCollectionsForMediaItemRequest) (*eveyv1.ListCollectionsForMediaItemResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.ListCollectionsForMediaItem")
	defer span.End()
	if req.GetMediaItemId() == "" {
		span.SetStatus(otelcodes.Error, "media_item_id is required")
		return nil, status.Errorf(codes.InvalidArgument, "media_item_id is required")
	}
	cols, err := h.svc.GetForMediaItem(ctx, req.GetMediaItemId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "collections for media item", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		span.RecordError(filterErr)
		span.SetStatus(otelcodes.Error, filterErr.Error())
		return nil, internalError(ctx, "check access", filterErr)
	}
	resp := &eveyv1.ListCollectionsForMediaItemResponse{}
	for _, c := range cols {
		items, _, itemsErr := h.svc.GetItems(ctx, c.ID, library.MaxCollectionItems, 0)
		if itemsErr != nil {
			slog.WarnContext(ctx, "collections for media item: items lookup", slog.String("collection_id", c.ID), slog.Any("err", itemsErr))
			continue
		}
		visible, skip := filterItemsByAccess(items, allowed)
		if skip || !h.collectionVisibleUnderParental(ctx, visible) {
			continue
		}
		pb := collectionToProto(c)
		pb.ItemCount = int32(len(visible))
		resp.Collections = append(resp.Collections, pb)
	}
	return resp, nil
}

// RemoveCollectionItem is AddCollectionItem's missing counterpart: until it
// existed a collection could only ever grow, so a wrong member was permanent
// and hand-curation was write-only.
func (h *collectionHandler) RemoveCollectionItem(ctx context.Context, req *eveyv1.RemoveCollectionItemRequest) (*eveyv1.RemoveCollectionItemResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.RemoveCollectionItem")
	defer span.End()
	if req.GetId() == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	if req.GetMediaItemId() == "" {
		span.SetStatus(otelcodes.Error, "media_item_id is required")
		return nil, status.Errorf(codes.InvalidArgument, "media_item_id is required")
	}
	// Defense-in-depth IDOR check, like DeleteCollection: RBAC already limits
	// this to admins, but a collection the caller cannot see must stay
	// indistinguishable from one that does not exist.
	if err := h.checkCollectionAccess(ctx, req.GetId()); err != nil {
		span.SetStatus(otelcodes.Error, "collection access denied")
		return nil, err
	}
	if err := h.svc.UnlinkMediaItem(ctx, req.GetId(), req.GetMediaItemId()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "remove collection item", err)
	}
	return &eveyv1.RemoveCollectionItemResponse{}, nil
}

// ReorderCollectionItems stamps the hand-set order. It only changes what a
// viewer sees for a collection whose sort_by is "manual" - the caller owns
// that choice, so reordering never silently rewrites how a saga reads.
func (h *collectionHandler) ReorderCollectionItems(ctx context.Context, req *eveyv1.ReorderCollectionItemsRequest) (*eveyv1.ReorderCollectionItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.ReorderCollectionItems")
	defer span.End()
	if req.GetId() == "" {
		span.SetStatus(otelcodes.Error, "id is required")
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}
	if len(req.GetMediaItemIds()) == 0 {
		span.SetStatus(otelcodes.Error, "media_item_ids is required")
		return nil, status.Errorf(codes.InvalidArgument, "media_item_ids is required")
	}
	if len(req.GetMediaItemIds()) > library.MaxCollectionItems {
		span.SetStatus(otelcodes.Error, "too many items")
		return nil, status.Errorf(codes.InvalidArgument, "at most %d items", library.MaxCollectionItems)
	}
	if err := h.checkCollectionAccess(ctx, req.GetId()); err != nil {
		span.SetStatus(otelcodes.Error, "collection access denied")
		return nil, err
	}
	// The reorder UPDATE matches on membership, so an id that is not in the
	// collection silently changes nothing: the caller got a 200 for a reorder
	// that half happened. Name the strangers instead of letting a client bug
	// look like success.
	if foreign, err := h.foreignCollectionItems(ctx, req.GetId(), req.GetMediaItemIds()); err != nil {
		span.RecordError(err)
		return nil, internalError(ctx, "load collection items for reorder", err)
	} else if len(foreign) > 0 {
		span.SetStatus(otelcodes.Error, "media_item_ids contains non-members")
		return nil, status.Errorf(codes.InvalidArgument, "media_item_ids contains items that are not in this collection: %s", strings.Join(foreign, ", "))
	}
	if err := h.svc.ReorderItems(ctx, req.GetId(), req.GetMediaItemIds()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "reorder collection items", err)
	}
	return &eveyv1.ReorderCollectionItemsResponse{}, nil
}

// foreignCollectionItems returns the requested ids that the collection does
// not actually hold, in request order and deduplicated. Empty means every id
// is a member.
func (h *collectionHandler) foreignCollectionItems(ctx context.Context, collectionID string, ids []string) ([]string, error) {
	items, _, err := h.svc.GetItems(ctx, collectionID, library.MaxCollectionItems, 0)
	if err != nil {
		return nil, err
	}
	members := make(map[string]struct{}, len(items))
	for _, it := range items {
		members[string(it.ID)] = struct{}{}
	}
	var foreign []string
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := members[id]; ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		foreign = append(foreign, id)
	}
	return foreign, nil
}

func protoRulesToDomain(in []*eveyv1.CollectionRule) []library.CollectionRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]library.CollectionRule, 0, len(in))
	for _, r := range in {
		if r == nil {
			continue
		}
		out = append(out, library.CollectionRule{
			Field:    r.GetField(),
			Operator: r.GetOperator(),
			Value:    r.GetValue(),
		})
	}
	return out
}

func (h *collectionHandler) GetCollectionItems(ctx context.Context, req *eveyv1.GetCollectionItemsRequest) (*eveyv1.GetCollectionItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.GetCollectionItems")
	defer span.End()
	// Clamp pagination: a negative offset would reach the repo as OFFSET -1 and
	// error out (500); limit<=0 is normalised so the page size is well-defined.
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := max(int(req.Offset), 0)
	items, total, err := h.svc.GetItems(ctx, req.Id, limit, offset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get collection items", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		span.RecordError(filterErr)
		span.SetStatus(otelcodes.Error, filterErr.Error())
		return nil, internalError(ctx, "check access", filterErr)
	}
	// Apply the active profile's parental filter per item - without this a kid
	// profile could enumerate adult titles through GetCollectionItems even
	// though GetCollection/ListCollections hide the collection.
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	parentalActive := pf.maxAge >= 0 || len(pf.blockedTags) > 0
	ids := make([]string, 0, len(items))
	enriched := make([]*eveyv1.CollectionItemProto, 0, len(items))
	for _, it := range items {
		if allowed != nil && !slices.Contains(allowed, string(it.LibraryID)) {
			total--
			continue
		}
		if parentalActive && mediaItemBlockedByParental(pf, it) {
			total--
			continue
		}
		ids = append(ids, string(it.ID))
		enriched = append(enriched, collectionItemToProto(it))
	}
	return &eveyv1.GetCollectionItemsResponse{
		MediaItemIds: ids,
		Total:        int32(total),
		Items:        enriched,
	}, nil
}

// defaultCollectionPageSize matches what GetCollectionItems serves, so the two
// ways into a collection agree on how much they hand back.
const defaultCollectionPageSize = 50

// collectionItemToProto is shared by both ways of reading a collection's
// members: the detail response and the paged item listing must describe the
// same title identically.
func collectionItemToProto(it *library.MediaItem) *eveyv1.CollectionItemProto {
	ci := &eveyv1.CollectionItemProto{
		Id:       string(it.ID),
		Title:    it.Title,
		Type:     string(it.Type),
		Overview: it.Overview,
	}
	if it.Catalog != nil {
		ci.Genres = it.Catalog.Genres
		if it.Catalog.Year != nil {
			ci.Year = int32(*it.Catalog.Year)
		}
	}
	if it.Classification != nil && it.Classification.CommunityRating != nil {
		ci.CommunityRating = *it.Classification.CommunityRating
	}
	return ci
}

func itemsToProto(items []*library.MediaItem) []*eveyv1.CollectionItemProto {
	out := make([]*eveyv1.CollectionItemProto, 0, len(items))
	for _, it := range items {
		out = append(out, collectionItemToProto(it))
	}
	return out
}

func (h *collectionHandler) allowedLibraryIDs(ctx context.Context) ([]string, error) {
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	if h.userAccess == nil {
		// nil here would be read by every caller as "unrestricted" (see the
		// comment below): a wiring regression would silently show every
		// collection to every non-admin. A non-nil empty slice keeps the same
		// "denies everything" behaviour as a real zero-access user.
		return []string{}, nil
	}
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil // zero grants = unrestricted (opt-out default)
	}
	return ids, nil
}

// checkCollectionAccess verifies the caller can interact with the collection
// identified by collectionID. Admins short-circuit. Non-admins must have
// library access on at least one media item of the collection; otherwise
// NotFound is returned to avoid leaking existence. A handler with userAccess
// unwired denies every non-admin the same way (see allowedLibraryIDs).
// Returns a gRPC status error suitable for direct propagation to the client.
func (h *collectionHandler) checkCollectionAccess(ctx context.Context, collectionID string) error {
	allowed, err := h.allowedLibraryIDs(ctx)
	if err != nil {
		return internalError(ctx, "check access", err)
	}
	if allowed == nil {
		// Admin or no access service wired (tests) - defer to RBAC.
		return nil
	}
	if _, getErr := h.svc.GetByID(ctx, collectionID); getErr != nil {
		return status.Errorf(codes.NotFound, "collection not found")
	}
	items, _, itemsErr := h.svc.GetItems(ctx, collectionID, library.MaxCollectionItems, 0)
	if itemsErr != nil {
		slog.WarnContext(ctx, "collection access check: get items", slog.String("collection_id", collectionID), slog.Any("err", itemsErr))
		return internalError(ctx, "get collection items", itemsErr)
	}
	for _, it := range items {
		if slices.Contains(allowed, string(it.LibraryID)) {
			return nil
		}
	}
	return status.Errorf(codes.NotFound, "collection not found")
}

// collectionKind names how a collection came to exist: sourced from a TMDB
// collection, driven by rules, or hand-built.
func collectionKind(c *library.Collection) string {
	switch {
	case c.TmdbID != nil:
		return "saga"
	case len(c.Rules) > 0:
		return "smart"
	default:
		return "manual"
	}
}

func collectionToProto(c *library.Collection) *eveyv1.CollectionProto {
	pb := &eveyv1.CollectionProto{
		Id:          c.ID,
		Name:        c.Name,
		Overview:    c.Overview,
		PosterUrl:   c.PosterURL,
		BackdropUrl: c.BackdropURL,
		CreatedAt:   toTimestamppb(c.CreatedAt),
		UpdatedAt:   toTimestamppb(c.UpdatedAt),
		SortBy:      c.SortBy,
		Kind:        collectionKind(c),
	}
	if c.TmdbID != nil {
		v := int32(*c.TmdbID)
		pb.TmdbId = &v
	}
	if len(c.Rules) > 0 {
		pb.Rules = make([]*eveyv1.CollectionRule, 0, len(c.Rules))
		for _, r := range c.Rules {
			pb.Rules = append(pb.Rules, &eveyv1.CollectionRule{
				Field:    r.Field,
				Operator: r.Operator,
				Value:    r.Value,
			})
		}
	}
	return pb
}
