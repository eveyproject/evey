package grpcapi

import (
	"context"
	"slices"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type collectionHandler struct {
	eveyv1.UnimplementedCollectionServiceServer
	svc        *appcollection.Service
	userAccess *appuser.AccessService
}

func (h *collectionHandler) ListCollections(ctx context.Context, _ *eveyv1.ListCollectionsRequest) (*eveyv1.ListCollectionsResponse, error) {
	cols, err := h.svc.List(ctx)
	if err != nil {
		return nil, internalError("list collections", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		return nil, internalError("check access", filterErr)
	}
	resp := &eveyv1.ListCollectionsResponse{}
	for _, c := range cols {
		if allowed != nil {
			items, _, itemsErr := h.svc.GetItems(ctx, c.ID, 1000, 0)
			if itemsErr != nil {
				continue
			}
			visible := false
			for _, it := range items {
				if slices.Contains(allowed, string(it.LibraryID)) {
					visible = true
					break
				}
			}
			if !visible {
				continue
			}
		}
		resp.Collections = append(resp.Collections, collectionToProto(c))
	}
	return resp, nil
}

func (h *collectionHandler) GetCollection(ctx context.Context, req *eveyv1.GetCollectionRequest) (*eveyv1.GetCollectionResponse, error) {
	col, err := h.svc.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "collection not found: %v", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
		return nil, internalError("check access", filterErr)
	}
	if allowed != nil {
		items, _, itemsErr := h.svc.GetItems(ctx, col.ID, 1000, 0)
		if itemsErr != nil {
			return nil, internalError("get collection items", itemsErr)
		}
		visible := false
		for _, it := range items {
			if slices.Contains(allowed, string(it.LibraryID)) {
				visible = true
				break
			}
		}
		if !visible {
			return nil, status.Errorf(codes.NotFound, "collection not found")
		}
	}
	return &eveyv1.GetCollectionResponse{Collection: collectionToProto(col)}, nil
}

func (h *collectionHandler) DeleteCollection(ctx context.Context, req *eveyv1.DeleteCollectionRequest) (*eveyv1.DeleteCollectionResponse, error) {
	if err := h.svc.Delete(ctx, req.Id); err != nil {
		return nil, internalError("delete collection", err)
	}
	return &eveyv1.DeleteCollectionResponse{}, nil
}

func (h *collectionHandler) GetCollectionItems(ctx context.Context, req *eveyv1.GetCollectionItemsRequest) (*eveyv1.GetCollectionItemsResponse, error) {
	items, total, err := h.svc.GetItems(ctx, req.Id, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, internalError("get collection items", err)
	}
	allowed, filterErr := h.allowedLibraryIDs(ctx)
	if filterErr != nil {
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
			Genres:   it.Genres,
		}
		if it.Year != nil {
			ci.Year = int32(*it.Year)
		}
		if it.CommunityRating != nil {
			ci.CommunityRating = *it.CommunityRating
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
