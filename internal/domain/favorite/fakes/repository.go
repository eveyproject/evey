package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/favorite"
)

type FakeRepository struct {
	mu    sync.RWMutex
	items map[string]*favorite.Favorite // key: profileID:mediaItemID
	Err   error
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{items: make(map[string]*favorite.Favorite)}
}

func key(profileID, mediaItemID string) string { return profileID + ":" + mediaItemID }

func (r *FakeRepository) Add(_ context.Context, fav *favorite.Favorite) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[key(fav.ProfileID, fav.MediaItemID)] = fav
	return nil
}

func (r *FakeRepository) Remove(_ context.Context, profileID, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, key(profileID, mediaItemID))
	return nil
}

func (r *FakeRepository) List(_ context.Context, profileID string, limit, offset int) ([]*favorite.Favorite, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*favorite.Favorite
	for _, f := range r.items {
		if f.ProfileID == profileID {
			result = append(result, f)
		}
	}
	if offset > 0 && offset < len(result) {
		result = result[offset:]
	} else if offset >= len(result) {
		return nil, nil
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *FakeRepository) IsFavorite(_ context.Context, profileID, mediaItemID string) (bool, error) {
	if r.Err != nil {
		return false, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[key(profileID, mediaItemID)]
	return ok, nil
}
