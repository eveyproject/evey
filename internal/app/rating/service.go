package rating

import (
	"context"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// Service exposes per-profile thumb ratings (like / dislike / clear, value
// -1/0/1) backed by the user_ratings table. The legacy 1-10 star-rating system
// (the domain/rating package + the ratings table + RatingValue) was removed -
// thumbs is the single rating model.
type Service struct {
	thumb domainlibrary.UserRatingRepository
}

func NewService(thumb domainlibrary.UserRatingRepository) *Service {
	return &Service{thumb: thumb}
}

func (s *Service) UpsertThumb(ctx context.Context, r *domainlibrary.UserRating) error {
	return s.thumb.Upsert(ctx, r)
}

func (s *Service) GetThumb(ctx context.Context, profileID, mediaItemID string) (*domainlibrary.UserRating, error) {
	return s.thumb.Get(ctx, profileID, mediaItemID)
}
