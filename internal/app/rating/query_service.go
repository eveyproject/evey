package rating

import (
	"context"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/rating"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type QueryService struct {
	repo rating.Repository
}

func NewQueryService(repo rating.Repository) *QueryService {
	return &QueryService{repo: repo}
}

func (s *QueryService) GetRating(ctx context.Context, mediaItemID, profileID string) (*rating.Rating, error) {
	rt, err := s.repo.Get(ctx, mediaItemID, profileID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rt, nil
}

func (s *QueryService) GetAggregates(ctx context.Context, mediaItemIDs []string) (map[string]rating.Aggregate, error) {
	return s.repo.GetAggregates(ctx, mediaItemIDs)
}
