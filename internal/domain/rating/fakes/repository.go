package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/rating"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeRepository struct {
	mu      sync.RWMutex
	ratings map[string]rating.Rating // key: mediaItemID:profileID
	Err     error
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{ratings: make(map[string]rating.Rating)}
}

func key(mediaItemID, profileID string) string { return mediaItemID + ":" + profileID }

func (r *FakeRepository) Upsert(_ context.Context, rt rating.Rating) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ratings[key(string(rt.MediaItemID), string(rt.ProfileID))] = rt
	return nil
}

func (r *FakeRepository) Get(_ context.Context, mediaItemID, profileID string) (*rating.Rating, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.ratings[key(mediaItemID, profileID)]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return &rt, nil
}

func (r *FakeRepository) Delete(_ context.Context, mediaItemID, profileID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ratings, key(mediaItemID, profileID))
	return nil
}

func (r *FakeRepository) GetAggregates(_ context.Context, mediaItemIDs []string) (map[string]rating.Aggregate, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	// group by mediaItemID
	type sum struct{ total, count int }
	sums := make(map[string]sum)
	for _, rt := range r.ratings {
		s := sums[string(rt.MediaItemID)]
		s.total += rt.Value.Int()
		s.count++
		sums[string(rt.MediaItemID)] = s
	}
	result := make(map[string]rating.Aggregate, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if s, ok := sums[id]; ok {
			result[id] = rating.Aggregate{Average: float64(s.total) / float64(s.count), Count: s.count}
		}
	}
	return result, nil
}
