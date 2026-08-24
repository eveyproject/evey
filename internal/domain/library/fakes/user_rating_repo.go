package fakes

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeUserRatingRepository struct {
	ratings map[string]*library.UserRating // key: profileID+":"+mediaItemID
	// Err makes every call fail, the way the postgres repo does when the
	// database is unreachable. Without it a test cannot tell "no rating" from
	// "could not read", which is precisely the confusion this repo's callers
	// have to get right.
	Err error
}

func NewFakeUserRatingRepository() *FakeUserRatingRepository {
	return &FakeUserRatingRepository{ratings: make(map[string]*library.UserRating)}
}

func (r *FakeUserRatingRepository) Upsert(_ context.Context, rating *library.UserRating) error {
	if r.Err != nil {
		return r.Err
	}
	r.ratings[string(rating.ProfileID)+":"+string(rating.MediaItemID)] = rating
	return nil
}

func (r *FakeUserRatingRepository) Get(_ context.Context, profileID, mediaItemID string) (*library.UserRating, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	ur, ok := r.ratings[profileID+":"+mediaItemID]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return ur, nil
}

func (r *FakeUserRatingRepository) GetForItems(_ context.Context, profileID string, mediaItemIDs []string) (map[string]*library.UserRating, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[string]*library.UserRating)
	for _, id := range mediaItemIDs {
		if ur, ok := r.ratings[profileID+":"+id]; ok {
			result[id] = ur
		}
	}
	return result, nil
}
