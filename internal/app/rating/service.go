package rating

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gravity-zero/evey/internal/domain/rating"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type Service struct {
	repo rating.Repository
}

func NewService(repo rating.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Rate(ctx context.Context, mediaItemID, profileID string, value int) error {
	if value < 1 || value > 5 {
		return fmt.Errorf("value must be between 1 and 5")
	}
	now := time.Now()
	return s.repo.Upsert(ctx, rating.Rating{
		MediaItemID: mediaItemID,
		ProfileID:   profileID,
		Value:       value,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func (s *Service) GetRating(ctx context.Context, mediaItemID, profileID string) (*rating.Rating, error) {
	rt, err := s.repo.Get(ctx, mediaItemID, profileID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rt, nil
}

func (s *Service) DeleteRating(ctx context.Context, mediaItemID, profileID string) error {
	return s.repo.Delete(ctx, mediaItemID, profileID)
}

func (s *Service) GetAggregates(ctx context.Context, mediaItemIDs []string) (map[string]rating.Aggregate, error) {
	return s.repo.GetAggregates(ctx, mediaItemIDs)
}
