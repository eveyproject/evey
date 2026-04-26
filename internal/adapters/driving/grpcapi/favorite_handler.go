package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appfavorite "github.com/evey-project/evey/internal/app/favorite"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type favoriteHandler struct {
	eveyv1.UnimplementedFavoriteServiceServer
	svc        *appfavorite.Service
	libQuery   *applibrary.QueryService
	userAccess *appuser.AccessService
}

func (h *favoriteHandler) requireMediaAccess(ctx context.Context, mediaItemID string) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if claims.IsAdmin || h.userAccess == nil {
		return nil
	}
	item, err := h.libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return status.Error(codes.NotFound, "not found")
	}
	ok, accessErr := h.userAccess.HasLibraryAccess(ctx, claims.UserID, string(item.LibraryID))
	if accessErr != nil || !ok {
		return status.Error(codes.NotFound, "not found")
	}
	return nil
}

func (h *favoriteHandler) ListFavorites(ctx context.Context, req *eveyv1.ListFavoritesRequest) (*eveyv1.ListFavoritesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.favorite.ListFavorites")
	defer span.End()
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
		return nil, internalError("list favorites", err)
	}
	itemByID := make(map[string]*library.MediaItem)
	if h.libQuery != nil {
		ids := make([]string, 0, len(favs))
		for _, f := range favs {
			ids = append(ids, string(f.MediaItemID))
		}
		items, _ := h.libQuery.GetMediaItemsByIDs(ctx, ids)
		for _, it := range items {
			itemByID[string(it.ID)] = it
		}
	}
	allowed := func(libraryID string) bool { return true }
	if !claims.IsAdmin && h.userAccess != nil {
		access, _ := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
		accessSet := make(map[string]struct{}, len(access))
		for _, libID := range access {
			accessSet[libID] = struct{}{}
		}
		allowed = func(libraryID string) bool { _, ok := accessSet[libraryID]; return ok }
	}
	resp := &eveyv1.ListFavoritesResponse{}
	for _, f := range favs {
		fp := &eveyv1.FavoriteProto{
			Id:          f.ID,
			ProfileId:   string(f.ProfileID),
			MediaItemId: string(f.MediaItemID),
			CreatedAt:   toTimestamppb(f.CreatedAt),
		}
		if it := itemByID[string(f.MediaItemID)]; it != nil {
			if !allowed(string(it.LibraryID)) {
				continue
			}
			fp.Title = it.Title
			fp.Type = string(it.Type)
			if it.Catalog != nil && it.Catalog.Year != nil {
				fp.Year = int32(*it.Catalog.Year)
			}
			if it.Classification != nil && it.Classification.CommunityRating != nil {
				fp.CommunityRating = *it.Classification.CommunityRating
			}
		}
		resp.Items = append(resp.Items, fp)
	}
	return resp, nil
}

func (h *favoriteHandler) AddFavorite(ctx context.Context, req *eveyv1.AddFavoriteRequest) (*eveyv1.AddFavoriteResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.favorite.AddFavorite")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.AddFavorite: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.AddFavorite: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("favoriteHandler.AddFavorite: %w", accessErr)
	}
	isFav, err := h.svc.Toggle(ctx, profileID, req.MediaItemId)
	if err != nil {
		return nil, internalError("toggle favorite", err)
	}
	return &eveyv1.AddFavoriteResponse{IsFavorite: isFav}, nil
}

func (h *favoriteHandler) RemoveFavorite(ctx context.Context, req *eveyv1.RemoveFavoriteRequest) (*eveyv1.RemoveFavoriteResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.favorite.RemoveFavorite")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.RemoveFavorite: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.RemoveFavorite: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("favoriteHandler.RemoveFavorite: %w", accessErr)
	}
	isFav, err := h.svc.IsFavorite(ctx, profileID, req.MediaItemId)
	if err != nil {
		return nil, internalError("check favorite", err)
	}
	if isFav {
		if _, err := h.svc.Toggle(ctx, profileID, req.MediaItemId); err != nil {
			return nil, internalError("remove favorite", err)
		}
	}
	return &eveyv1.RemoveFavoriteResponse{}, nil
}

func (h *favoriteHandler) IsFavorite(ctx context.Context, req *eveyv1.IsFavoriteRequest) (*eveyv1.IsFavoriteResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.favorite.IsFavorite")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.IsFavorite: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.IsFavorite: %w", err)
	}
	if accessErr := h.requireMediaAccess(ctx, req.MediaItemId); accessErr != nil {
		return nil, fmt.Errorf("favoriteHandler.IsFavorite: %w", accessErr)
	}
	isFav, err := h.svc.IsFavorite(ctx, profileID, req.MediaItemId)
	if err != nil {
		return nil, internalError("check favorite", err)
	}
	return &eveyv1.IsFavoriteResponse{IsFavorite: isFav}, nil
}
