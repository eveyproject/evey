package rating

import (
	"context"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/rating"
	"github.com/evey-project/evey/internal/domain/shared"
)

// Service is a facade over CommandService and QueryService for backward compatibility.
type Service struct {
	cmd   *CommandService
	qry   *QueryService
	thumb domainlibrary.UserRatingRepository
}

type ServiceOption func(*Service)

func WithThumbRating(repo domainlibrary.UserRatingRepository) ServiceOption {
	return func(s *Service) { s.thumb = repo }
}

func NewService(repo rating.Repository, bus shared.EventBus, opts ...ServiceOption) *Service {
	s := &Service{
		cmd: NewCommandService(repo, bus),
		qry: NewQueryService(repo),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

func (s *Service) UpsertThumb(ctx context.Context, r *domainlibrary.UserRating) error {
	return s.thumb.Upsert(ctx, r)
}

func (s *Service) GetThumb(ctx context.Context, profileID, mediaItemID string) (*domainlibrary.UserRating, error) {
	return s.thumb.Get(ctx, profileID, mediaItemID)
}
