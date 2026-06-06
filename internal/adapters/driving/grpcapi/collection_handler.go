package grpcapi

import (
	"context"
	"log/slog"
	"slices"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
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

func (h *collectionHandler) ListCollections(ctx context.Context, _ *eveyv1.ListCollectionsRequest) (*eveyv1.ListCollectionsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.collection.ListCollections")
	defer span.End()
	cols, err := h.svc.List(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("list collections", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		span.RecordError(filterErr)
		span.SetStatus(otelcodes.Error, filterErr.Error())
		return nil, internalError("check access", filterErr)
	}
	resp := &eveyv1.ListCollectionsResponse{}
	for _, c := range cols {
		items, itemsErr := h.collectionItemsForFilter(ctx, c.ID, allowed)
		if itemsErr {
			continue
		}
		if !h.collectionVisibleUnderParental(ctx, items) {
			continue
		}
		resp.Collections = append(resp.Collections, collectionToProto(c))
	}
	return resp, nil
}

// collectionItemsForFilter resolves the items of a collection and returns
// only those the caller has library access to. The bool is set when the
// collection should be skipped (item lookup failed, or library access
// rules eliminate every item).
func (h *collectionHandler) collectionItemsForFilter(ctx context.Context, collectionID string, allowedLibIDs []string) ([]*library.MediaItem, bool) {
	items, _, itemsErr := h.svc.GetItems(ctx, collectionID, 1000, 0)
	if itemsErr != nil {
		return nil, true
	}
	if allowedLibIDs == nil {
		return items, false
	}
	visible := items[:0]
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
		return nil, internalError("check access", filterErr)
	}
	items, _, itemsErr := h.svc.GetItems(ctx, col.ID, 1000, 0)
	if itemsErr != nil {
		span.RecordError(itemsErr)
		span.SetStatus(otelcodes.Error, itemsErr.Error())
		return nil, internalError("get collection items", itemsErr)
	}
	if allowed != nil {
		visible := false
		for _, it := range items {
			if slices.Contains(allowed, string(it.LibraryID)) {
				visible = true
				break
			}
		}
		if !visible {
			span.SetStatus(otelcodes.Error, "collection access denied")
			return nil, status.Errorf(codes.NotFound, "collection not found")
		}
	}
	if !h.collectionVisibleUnderParental(ctx, items) {
		span.SetStatus(otelcodes.Error, "collection blocked by parental filter")
		return nil, status.Errorf(codes.NotFound, "collection not found")
	}
	return &eveyv1.GetCollectionResponse{Collection: collectionToProto(col)}, nil
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
		return nil, internalError("delete collection", err)
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
		created, err = h.svc.Create(ctx, req.GetName(), rules, req.GetSortBy())
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("create collection", err)
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
			return nil, internalError("update smart collection", err)
		}
		return &eveyv1.UpdateCollectionResponse{Collection: collectionToProto(col)}, nil
	}
	if err := h.svc.Update(ctx, req.GetId(), req.GetName(), rules, req.GetSortBy()); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("update collection", err)
	}
	col, err := h.svc.GetByID(ctx, req.GetId())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError("fetch updated collection", err)
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
		return nil, internalError("add collection item", err)
	}
	return &eveyv1.AddCollectionItemResponse{}, nil
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
		return nil, internalError("get collection items", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		span.RecordError(filterErr)
		span.SetStatus(otelcodes.Error, filterErr.Error())
		return nil, internalError("check access", filterErr)
	}
	ids := make([]string, 0, len(items))
	enriched := make([]*eveyv1.CollectionItemProto, 0, len(items))
	for _, it := range items {
		if allowed != nil && !slices.Contains(allowed, string(it.LibraryID)) {
			total--
			continue
		}
		ids = append(ids, string(it.ID))
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
		enriched = append(enriched, ci)
	}
	return &eveyv1.GetCollectionItemsResponse{
		MediaItemIds: ids,
		Total:        int32(total),
		Items:        enriched,
	}, nil
}

func (h *collectionHandler) allowedLibraryIDs(ctx context.Context) ([]string, error) {
	if h.userAccess == nil {
		return nil, nil
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	return h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
}

// checkCollectionAccess verifies the caller can interact with the collection
// identified by collectionID. Admins (and unauthenticated test contexts where
// userAccess is nil) short-circuit. Non-admins must have library access on at
// least one media item of the collection; otherwise NotFound is returned to
// avoid leaking existence. Returns a gRPC status error suitable for direct
// propagation to the client.
func (h *collectionHandler) checkCollectionAccess(ctx context.Context, collectionID string) error {
	allowed, err := h.allowedLibraryIDs(ctx)
	if err != nil {
		return internalError("check access", err)
	}
	if allowed == nil {
		// Admin or no access service wired (tests) — defer to RBAC.
		return nil
	}
	if _, getErr := h.svc.GetByID(ctx, collectionID); getErr != nil {
		return status.Errorf(codes.NotFound, "collection not found")
	}
	items, _, itemsErr := h.svc.GetItems(ctx, collectionID, 1000, 0)
	if itemsErr != nil {
		slog.Warn("collection access check: get items", slog.String("collection_id", collectionID), slog.Any("err", itemsErr))
		return internalError("get collection items", itemsErr)
	}
	for _, it := range items {
		if slices.Contains(allowed, string(it.LibraryID)) {
			return nil
		}
	}
	return status.Errorf(codes.NotFound, "collection not found")
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
