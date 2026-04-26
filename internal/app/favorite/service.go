package favorite

import (
	"context"

	"github.com/evey-project/evey/internal/domain/favorite"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type Service struct {
	repo favorite.Repository
}

func NewService(repo favorite.Repository) *Service {
	return &Service{repo: repo}
}

// Toggle adds if not favorited, removes if already favorited. Returns new state.
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
	fav, err := favorite.NewFavorite(id.New("fav-"), shared.ProfileID(profileID), shared.MediaItemID(mediaItemID))
	if err != nil {
		return false, err
	}
	return true, s.repo.Add(ctx, fav)
}

func (s *Service) List(ctx context.Context, profileID string, limit, offset int) ([]*favorite.Favorite, error) {
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
