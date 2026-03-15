package rating

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/rating"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

// Service is a facade over CommandService and QueryService for backward compatibility.
type Service struct {
	cmd *CommandService
	qry *QueryService
}

func NewService(repo rating.Repository, bus shared.EventBus) *Service {
	return &Service{
		cmd: NewCommandService(repo, bus),
		qry: NewQueryService(repo),
	}
}

func (s *Service) Rate(ctx context.Context, mediaItemID, profileID string, value int) error {
	return s.cmd.Rate(ctx, mediaItemID, profileID, value)
}

func (s *Service) DeleteRating(ctx context.Context, mediaItemID, profileID string) error {
	return s.cmd.DeleteRating(ctx, mediaItemID, profileID)
}

func (s *Service) GetRating(ctx context.Context, mediaItemID, profileID string) (*rating.Rating, error) {
	return s.qry.GetRating(ctx, mediaItemID, profileID)
}

func (s *Service) GetAggregates(ctx context.Context, mediaItemIDs []string) (map[string]rating.Aggregate, error) {
	return s.qry.GetAggregates(ctx, mediaItemIDs)
}
