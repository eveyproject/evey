package rating

import (
	"context"

	"github.com/evey-project/evey/internal/domain/rating"
)

type QueryService struct {
	repo rating.Repository
}

func NewQueryService(repo rating.Repository) *QueryService {
	return &QueryService{repo: repo}
}

// GetRating returns the rating or shared.ErrNotFound if absent.
func (s *QueryService) GetRating(ctx context.Context, mediaItemID, profileID string) (*rating.Rating, error) {
	return s.repo.Get(ctx, mediaItemID, profileID)
}

func (s *QueryService) GetAggregates(ctx context.Context, mediaItemIDs []string) (map[string]rating.Aggregate, error) {
	return s.repo.GetAggregates(ctx, mediaItemIDs)
}
