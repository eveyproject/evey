package favorite

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type Service struct {
	repo library.FavoriteRepository
}

func NewService(repo library.FavoriteRepository) *Service {
	return &Service{repo: repo}
}

// Toggle adds if not favorited, removes if already favorited. Returns new state.
//
// Kept for backward compatibility with existing callers: it reads current
// state then flips it, which is NOT safe against a duplicate call (a client
// retry after a lost response, or two concurrent requests) - two such calls
// can race the read and both flip, net-cancelling each other and silently
// leaving the item in the opposite state the caller wanted. New callers that
// know the state they want (add vs remove) should use SetFavorite instead,
// which is immune to that race.
func (s *Service) Toggle(ctx context.Context, profileID, mediaItemID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "app.favorite.Toggle")
	defer span.End()
	isFav, err := s.repo.IsFavorite(ctx, profileID, mediaItemID)
	if err != nil {
		return false, err
	}
	if isFav {
		return false, s.repo.Remove(ctx, profileID, mediaItemID)
	}
	fav, err := library.NewFavorite(id.New("fav-"), shared.ProfileID(profileID), shared.MediaItemID(mediaItemID))
	if err != nil {
		return false, err
	}
	return true, s.repo.Add(ctx, fav)
}

// SetFavorite makes the favorite state match want, without ever reading
// current state first. It goes straight to the repo's own idempotent
// primitives - Add is INSERT ... ON CONFLICT DO NOTHING, Remove is a DELETE
// that no-ops when the row is already gone - so calling it twice (or
// concurrently) with the same want always converges to the same result.
// This is what makes AddFavorite/RemoveFavorite safe against a client retry
// after a timeout: unlike Toggle, there is no read-then-flip window where a
// duplicate call can invert the outcome.
func (s *Service) SetFavorite(ctx context.Context, profileID, mediaItemID string, want bool) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "app.favorite.SetFavorite")
	defer span.End()
	if !want {
		return false, s.repo.Remove(ctx, profileID, mediaItemID)
	}
	fav, err := library.NewFavorite(id.New("fav-"), shared.ProfileID(profileID), shared.MediaItemID(mediaItemID))
	if err != nil {
		return false, err
	}
	return true, s.repo.Add(ctx, fav)
}

func (s *Service) List(ctx context.Context, profileID string, limit, offset int) ([]*library.Favorite, error) {
	ctx, span := tracing.StartSpan(ctx, "app.favorite.List")
	defer span.End()
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return s.repo.List(ctx, profileID, limit, offset)
}

func (s *Service) IsFavorite(ctx context.Context, profileID, mediaItemID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "app.favorite.IsFavorite")
	defer span.End()
	return s.repo.IsFavorite(ctx, profileID, mediaItemID)
}

// IsFavoriteMany answers for a whole screen of media items in one round trip.
// The returned set holds only favorited ids; an absent id is not favorited.
func (s *Service) IsFavoriteMany(ctx context.Context, profileID string, mediaItemIDs []string) (map[string]bool, error) {
	ctx, span := tracing.StartSpan(ctx, "app.favorite.IsFavoriteMany")
	defer span.End()
	if len(mediaItemIDs) == 0 {
		return map[string]bool{}, nil
	}
	return s.repo.IsFavoriteMany(ctx, profileID, mediaItemIDs)
}
