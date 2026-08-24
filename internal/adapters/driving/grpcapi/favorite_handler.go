package grpcapi

import (
	"context"
	"fmt"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
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
	userQuery  *appuser.QueryService
}

// requireMediaAccess delegates to the shared guard: favourites, playlists and
// ratings all ask the same question, and keeping three bodies is how favourite
// and rating drifted apart on where the admin short-circuit sits.
func (h *favoriteHandler) requireMediaAccess(ctx context.Context, mediaItemID string) error {
	return requireMediaLibraryAccess(ctx, mediaItemID, h.libQuery, h.userAccess, "favoriteHandler.requireMediaAccess")
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
	limit, offset := clampLimitOffset(req.Limit, req.Offset, 50)
	favs, err := h.svc.List(ctx, profileID, limit, offset)
	if err != nil {
		return nil, internalError(ctx, "list favorites", err)
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
		access, aErr := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
		// Fail closed: swallowing this error left access nil, which the check
		// below reads as "unrestricted" (nil = opt-out default) → on a DB blip
		// a non-admin would see the metadata of favorites in libraries they no
		// longer have access to. Mirror popular-media / activity.
		if aErr != nil {
			return nil, internalError(ctx, "list favorites: library access", aErr)
		}
		// Zero grants = unrestricted (opt-out default): only restrict when the
		// user has an explicit whitelist.
		if len(access) > 0 {
			accessSet := make(map[string]struct{}, len(access))
			for _, libID := range access {
				accessSet[libID] = struct{}{}
			}
			allowed = func(libraryID string) bool { _, ok := accessSet[libraryID]; return ok }
		}
	}
	// Parental: a kid profile must not see adult favorites' metadata (title /
	// year / rating) - drop blocked items entirely, mirroring resume/next-up.
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
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
			if mediaItemBlockedByParental(pf, it) {
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
	// State-setting, not toggling. A read-then-toggle looks idempotent but is
	// not: two concurrent double-taps can both read "not favorited" and both
	// flip, cancelling out. SetFavorite writes the target state directly and the
	// repo is idempotent underneath, so any number of concurrent calls converge.
	if _, err := h.svc.SetFavorite(ctx, profileID, req.MediaItemId, true); err != nil {
		return nil, internalError(ctx, "add favorite", err)
	}
	return &eveyv1.AddFavoriteResponse{IsFavorite: true}, nil
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
	// Same reasoning as AddFavorite: set the target state, do not toggle.
	if _, err := h.svc.SetFavorite(ctx, profileID, req.MediaItemId, false); err != nil {
		return nil, internalError(ctx, "remove favorite", err)
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
		return nil, internalError(ctx, "check favorite", err)
	}
	return &eveyv1.IsFavoriteResponse{IsFavorite: isFav}, nil
}

// maxBatchIsFavoriteIDs bounds one BatchIsFavorite call. Past it the call is
// refused, not truncated: a silently shortened answer reads as "not a favorite"
// on the client and paints an empty heart over a real favorite.
const maxBatchIsFavoriteIDs = 200

func (h *favoriteHandler) BatchIsFavorite(ctx context.Context, req *eveyv1.BatchIsFavoriteRequest) (*eveyv1.BatchIsFavoriteResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.favorite.BatchIsFavorite")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.BatchIsFavorite: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		return nil, fmt.Errorf("favoriteHandler.BatchIsFavorite: %w", err)
	}
	if len(req.MediaItemIds) > maxBatchIsFavoriteIDs {
		return nil, status.Errorf(codes.InvalidArgument, "media_item_ids: at most %d per call", maxBatchIsFavoriteIDs)
	}
	ids := dedupeNonEmpty(req.MediaItemIds)
	visible, err := h.visibleMediaItemIDs(ctx, claims.IsAdmin, ids)
	if err != nil {
		return nil, err
	}
	favs, err := h.svc.IsFavoriteMany(ctx, profileID, visible)
	if err != nil {
		return nil, internalError(ctx, "batch check favorite", err)
	}
	resp := &eveyv1.BatchIsFavoriteResponse{}
	for _, id := range visible {
		if favs[id] {
			resp.FavoriteMediaItemIds = append(resp.FavoriteMediaItemIds, id)
		}
	}
	return resp, nil
}

// visibleMediaItemIDs keeps only the ids the caller is allowed to learn about,
// in the order given. It is the batch counterpart of requireMediaAccess, with
// two differences that matter: it resolves every item in ONE query instead of
// one per id, and a refusal DROPS the id instead of raising NotFound - a batch
// answer must never let the caller tell "not a favorite" from "not yours" or
// "does not exist", or the RPC becomes an existence oracle over media ids.
func (h *favoriteHandler) visibleMediaItemIDs(ctx context.Context, isAdmin bool, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if h.libQuery == nil {
		// Without the catalog there is nothing to check access against. An admin
		// passes anyway (libraryAccessDecision would too); anyone else is refused
		// rather than served unguarded.
		if !isAdmin {
			return nil, errUserAccessNotConfigured(ctx, "favoriteHandler.BatchIsFavorite")
		}
		return ids, nil
	}
	items, err := h.libQuery.GetMediaItemsByIDs(ctx, ids)
	if err != nil {
		return nil, internalError(ctx, "batch check favorite: media items", err)
	}
	itemByID := make(map[string]*library.MediaItem, len(items))
	for _, it := range items {
		itemByID[string(it.ID)] = it
	}
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	// One decision per LIBRARY, not per item: a grid is a handful of libraries.
	allowedLib := make(map[string]bool, 4)
	visible := make([]string, 0, len(ids))
	for _, id := range ids {
		it := itemByID[id]
		if it == nil {
			continue
		}
		libID := string(it.LibraryID)
		allowed, known := allowedLib[libID]
		if !known {
			allowed, err = libraryAccessDecision(ctx, h.userAccess, libID, "favoriteHandler.BatchIsFavorite")
			if err != nil {
				// A missing port already carries its own status; anything else is
				// a lookup failure and must fail CLOSED, like ListFavorites.
				if status.Code(err) == codes.FailedPrecondition {
					return nil, err
				}
				return nil, internalError(ctx, "batch check favorite: library access", err)
			}
			allowedLib[libID] = allowed
		}
		if !allowed || mediaItemBlockedByParental(pf, it) {
			continue
		}
		visible = append(visible, id)
	}
	return visible, nil
}

func dedupeNonEmpty(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
