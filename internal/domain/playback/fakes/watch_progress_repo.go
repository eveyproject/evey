package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
)

var ErrNotFound = shared.ErrNotFound

type FakeWatchProgressRepository struct {
	mu    sync.RWMutex
	items map[string]*playback.WatchProgress
	Err   error
}

func NewFakeWatchProgressRepository() *FakeWatchProgressRepository {
	return &FakeWatchProgressRepository{items: make(map[string]*playback.WatchProgress)}
}

func key(profileID, mediaItemID string) string {
	return profileID + ":" + mediaItemID
}

func (r *FakeWatchProgressRepository) Upsert(_ context.Context, wp *playback.WatchProgress) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[key(string(wp.ProfileID), string(wp.MediaItemID))] = wp
	return nil
}

func (r *FakeWatchProgressRepository) GetByProfileAndMedia(_ context.Context, profileID, mediaItemID string) (*playback.WatchProgress, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	wp, ok := r.items[key(profileID, mediaItemID)]
	if !ok {
		return nil, ErrNotFound
	}
	return wp, nil
}

func (r *FakeWatchProgressRepository) GetByProfile(_ context.Context, profileID string, limit, offset int) ([]*playback.WatchProgress, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*playback.WatchProgress
	for _, wp := range r.items {
		if string(wp.ProfileID) == profileID {
			result = append(result, wp)
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

func (r *FakeWatchProgressRepository) GetResumeItems(_ context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*playback.WatchProgress
	for _, wp := range r.items {
		if string(wp.ProfileID) == profileID && !wp.Completed && wp.PositionMs > 0 {
			result = append(result, wp)
		}
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *FakeWatchProgressRepository) GetNextUp(_ context.Context, profileID string, limit int) ([]*playback.WatchProgress, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*playback.WatchProgress
	for _, wp := range r.items {
		if string(wp.ProfileID) == profileID && wp.Completed {
			result = append(result, wp)
		}
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}
