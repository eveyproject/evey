package grpcapi

import (
	"context"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	"github.com/gravity-zero/evey/internal/domain/library"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type collectionHandler struct {
	eveyv1.UnimplementedCollectionServiceServer
	svc *appcollection.Service
}

func (h *collectionHandler) ListCollections(ctx context.Context, _ *eveyv1.ListCollectionsRequest) (*eveyv1.ListCollectionsResponse, error) {
	cols, err := h.svc.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list collections: %v", err)
	}
	resp := &eveyv1.ListCollectionsResponse{}
	for _, c := range cols {
		resp.Collections = append(resp.Collections, collectionToProto(c))
	}
	return resp, nil
}

func (h *collectionHandler) GetCollection(ctx context.Context, req *eveyv1.GetCollectionRequest) (*eveyv1.CollectionProto, error) {
	col, err := h.svc.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}
	return collectionToProto(col), nil
}

func (h *collectionHandler) CreateCollection(ctx context.Context, req *eveyv1.CreateCollectionRequest) (*eveyv1.CollectionProto, error) {
	col := &library.Collection{
		Name:      req.Name,
		Overview:  req.Overview,
		Type:      collectionTypeFromProto(req.Type),
		Rules:     collectionRulesFromProto(req.Rules),
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}
	if err := h.svc.Create(ctx, col); err != nil {
		return nil, status.Errorf(codes.Internal, "create collection: %v", err)
	}
	return collectionToProto(col), nil
}

func (h *collectionHandler) UpdateCollection(ctx context.Context, req *eveyv1.UpdateCollectionRequest) (*eveyv1.CollectionProto, error) {
	col := &library.Collection{
		ID:        req.Id,
		Name:      req.Name,
		Overview:  req.Overview,
		Rules:     collectionRulesFromProto(req.Rules),
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}
	if err := h.svc.Update(ctx, col); err != nil {
		return nil, status.Errorf(codes.Internal, "update collection: %v", err)
	}
	updated, err := h.svc.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get updated collection: %v", err)
	}
	return collectionToProto(updated), nil
}

func (h *collectionHandler) DeleteCollection(ctx context.Context, req *eveyv1.DeleteCollectionRequest) (*eveyv1.DeleteCollectionResponse, error) {
	if err := h.svc.Delete(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete collection: %v", err)
	}
	return &eveyv1.DeleteCollectionResponse{}, nil
}

func (h *collectionHandler) GetCollectionItems(ctx context.Context, req *eveyv1.GetCollectionItemsRequest) (*eveyv1.GetCollectionItemsResponse, error) {
	items, total, err := h.svc.GetItems(ctx, req.Id, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get collection items: %v", err)
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return &eveyv1.GetCollectionItemsResponse{
		MediaItemIds: ids,
		Total:        int32(total),
	}, nil
}

func (h *collectionHandler) AddCollectionItem(ctx context.Context, req *eveyv1.AddCollectionItemRequest) (*eveyv1.AddCollectionItemResponse, error) {
	if err := h.svc.AddItem(ctx, req.Id, req.MediaItemId); err != nil {
		return nil, status.Errorf(codes.Internal, "add collection item: %v", err)
	}
	return &eveyv1.AddCollectionItemResponse{}, nil
}

func (h *collectionHandler) RemoveCollectionItem(ctx context.Context, req *eveyv1.RemoveCollectionItemRequest) (*eveyv1.RemoveCollectionItemResponse, error) {
	if err := h.svc.RemoveItem(ctx, req.Id, req.MediaItemId); err != nil {
		return nil, status.Errorf(codes.Internal, "remove collection item: %v", err)
	}
	return &eveyv1.RemoveCollectionItemResponse{}, nil
}

// --- converters ---

var collectionTypeToProtoMap = map[library.CollectionType]eveyv1.CollectionType{
	library.CollectionManual: eveyv1.CollectionType_COLLECTION_TYPE_MANUAL,
	library.CollectionSmart:  eveyv1.CollectionType_COLLECTION_TYPE_SMART,
}

var collectionTypeFromProtoMap = map[eveyv1.CollectionType]library.CollectionType{
	eveyv1.CollectionType_COLLECTION_TYPE_MANUAL: library.CollectionManual,
	eveyv1.CollectionType_COLLECTION_TYPE_SMART:  library.CollectionSmart,
}

func collectionTypeFromProto(t eveyv1.CollectionType) library.CollectionType {
	if ct, ok := collectionTypeFromProtoMap[t]; ok {
		return ct
	}
	return library.CollectionManual
}

func collectionToProto(c *library.Collection) *eveyv1.CollectionProto {
	pb := &eveyv1.CollectionProto{
		Id:        c.ID,
		Name:      c.Name,
		Overview:  c.Overview,
		Type:      collectionTypeToProtoMap[c.Type],
		SortBy:    c.SortBy,
		SortOrder: c.SortOrder,
		CreatedAt: toTimestamppb(c.CreatedAt),
		UpdatedAt: toTimestamppb(c.UpdatedAt),
	}
	for _, r := range c.Rules {
		pb.Rules = append(pb.Rules, &eveyv1.CollectionRule{
			Field:    r.Field,
			Operator: string(r.Operator),
			Value:    r.Value,
		})
	}
	return pb
}

func collectionRulesFromProto(rules []*eveyv1.CollectionRule) []library.CollectionRule {
	out := make([]library.CollectionRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, library.CollectionRule{
			Field:    r.Field,
			Operator: library.RuleOperator(r.Operator),
			Value:    r.Value,
		})
	}
	return out
}
