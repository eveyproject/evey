package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
)

type FakeFavoriteRepository struct {
	mu    sync.RWMutex
	items map[string]*library.Favorite // key: profileID:mediaItemID
	Err   error
}

func NewFakeFavoriteRepository() *FakeFavoriteRepository {
	return &FakeFavoriteRepository{items: make(map[string]*library.Favorite)}
}

func favoriteKey(profileID, mediaItemID string) string { return profileID + ":" + mediaItemID }

func (r *FakeFavoriteRepository) Add(_ context.Context, fav *library.Favorite) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[favoriteKey(string(fav.ProfileID), string(fav.MediaItemID))] = fav
	return nil
}

func (r *FakeFavoriteRepository) Remove(_ context.Context, profileID, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, favoriteKey(profileID, mediaItemID))
	return nil
}

func (r *FakeFavoriteRepository) List(_ context.Context, profileID string, limit, offset int) ([]*library.Favorite, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*library.Favorite
	for _, f := range r.items {
		if string(f.ProfileID) == profileID {
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

func (r *FakeFavoriteRepository) IsFavorite(_ context.Context, profileID, mediaItemID string) (bool, error) {
	if r.Err != nil {
		return false, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return false, r.Err
	}
	_, ok := r.items[favoriteKey(profileID, mediaItemID)]
	return ok, nil
}

func (r *FakeFavoriteRepository) IsFavoriteMany(_ context.Context, profileID string, mediaItemIDs []string) (map[string]bool, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	out := make(map[string]bool, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if _, ok := r.items[favoriteKey(profileID, id)]; ok {
			out[id] = true
		}
	}
	return out, nil
}
