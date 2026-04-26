package rating

import (
	"context"

	"github.com/evey-project/evey/internal/domain/rating"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type QueryService struct {
	repo rating.Repository
}

func NewQueryService(repo rating.Repository) *QueryService {
	return &QueryService{repo: repo}
}

// GetRating returns the rating or shared.ErrNotFound if absent.
func (s *QueryService) GetRating(ctx context.Context, mediaItemID, profileID string) (*rating.Rating, error) {
	ctx, span := tracing.StartSpan(ctx, "app.rating.GetRating")
	defer span.End()
	return s.repo.Get(ctx, mediaItemID, profileID)
}

func (s *QueryService) GetAggregates(ctx context.Context, mediaItemIDs []string) (map[string]rating.Aggregate, error) {
	ctx, span := tracing.StartSpan(ctx, "app.rating.GetAggregates")
	defer span.End()
	return s.repo.GetAggregates(ctx, mediaItemIDs)
}
