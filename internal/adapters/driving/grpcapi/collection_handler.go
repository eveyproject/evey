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

func (h *collectionHandler) GetCollection(ctx context.Context, req *eveyv1.GetCollectionRequest) (*eveyv1.GetCollectionResponse, error) {
	col, err := h.svc.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}
	return &eveyv1.GetCollectionResponse{Collection: collectionToProto(col)}, nil
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

func collectionToProto(c *library.Collection) *eveyv1.CollectionProto {
	pb := &eveyv1.CollectionProto{
		Id:          c.ID,
		Name:        c.Name,
		Overview:    c.Overview,
		PosterUrl:   c.PosterURL,
		BackdropUrl: c.BackdropURL,
		CreatedAt:   toTimestamppb(c.CreatedAt),
		UpdatedAt:   toTimestamppb(c.UpdatedAt),
	}
	if c.TmdbID != nil {
		v := int32(*c.TmdbID)
		pb.TmdbId = &v
	}
	return pb
}
