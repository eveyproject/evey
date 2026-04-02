package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type favoriteHandler struct {
	eveyv1.UnimplementedFavoriteServiceServer
	svc *appfavorite.Service
}

func (h *favoriteHandler) ListFavorites(ctx context.Context, req *eveyv1.ListFavoritesRequest) (*eveyv1.ListFavoritesResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.ListFavorites: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.ListFavorites: %w", err)
	}
	limit := int(req.Limit)
	offset := int(req.Offset)
	favs, err := h.svc.List(ctx, profileID, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list favorites: %v", err)
	}
	resp := &eveyv1.ListFavoritesResponse{}
	for _, f := range favs {
		resp.Items = append(resp.Items, &eveyv1.FavoriteProto{
			Id:          f.ID,
			ProfileId:   f.ProfileID,
			MediaItemId: f.MediaItemID,
			CreatedAt:   toTimestamppb(f.CreatedAt),
		})
	}
	return resp, nil
}

func (h *favoriteHandler) AddFavorite(ctx context.Context, req *eveyv1.AddFavoriteRequest) (*eveyv1.AddFavoriteResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.AddFavorite: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.AddFavorite: %w", err)
	}
	isFav, err := h.svc.Toggle(ctx, profileID, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "toggle favorite: %v", err)
	}
	return &eveyv1.AddFavoriteResponse{IsFavorite: isFav}, nil
}

func (h *favoriteHandler) RemoveFavorite(ctx context.Context, req *eveyv1.RemoveFavoriteRequest) (*eveyv1.RemoveFavoriteResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.RemoveFavorite: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.RemoveFavorite: %w", err)
	}
	isFav, err := h.svc.IsFavorite(ctx, profileID, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check favorite: %v", err)
	}
	if isFav {
		if _, err := h.svc.Toggle(ctx, profileID, req.MediaItemId); err != nil {
			return nil, status.Errorf(codes.Internal, "remove favorite: %v", err)
		}
	}
	return &eveyv1.RemoveFavoriteResponse{}, nil
}

func (h *favoriteHandler) IsFavorite(ctx context.Context, req *eveyv1.IsFavoriteRequest) (*eveyv1.IsFavoriteResponse, error) {
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.IsFavorite: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.IsFavorite: %w", err)
	}
	isFav, err := h.svc.IsFavorite(ctx, profileID, req.MediaItemId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check favorite: %v", err)
	}
	return &eveyv1.IsFavoriteResponse{IsFavorite: isFav}, nil
}
