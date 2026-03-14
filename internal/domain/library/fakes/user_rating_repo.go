package fakes

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type FakeUserRatingRepository struct {
	ratings map[string]*library.UserRating // key: profileID+":"+mediaItemID
}

func NewFakeUserRatingRepository() *FakeUserRatingRepository {
	return &FakeUserRatingRepository{ratings: make(map[string]*library.UserRating)}
}

func (r *FakeUserRatingRepository) Upsert(_ context.Context, rating *library.UserRating) error {
	r.ratings[rating.ProfileID+":"+rating.MediaItemID] = rating
	return nil
}

func (r *FakeUserRatingRepository) Get(_ context.Context, profileID, mediaItemID string) (*library.UserRating, error) {
	ur, ok := r.ratings[profileID+":"+mediaItemID]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return ur, nil
}

func (r *FakeUserRatingRepository) GetForItems(_ context.Context, profileID string, mediaItemIDs []string) (map[string]*library.UserRating, error) {
	result := make(map[string]*library.UserRating)
	for _, id := range mediaItemIDs {
		if ur, ok := r.ratings[profileID+":"+id]; ok {
			result[id] = ur
		}
	}
	return result, nil
}
