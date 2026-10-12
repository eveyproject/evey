package fakes

import (
	"context"
	"sort"
	"sync"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
)

var ErrNotFound = shared.ErrNotFound

type FakeWatchProgressRepository struct {
	mu    sync.RWMutex
	items map[string]*playback.WatchProgress
	Err   error
	// SeriesEpisodes answers ListSeriesEpisodes, by series id.
	SeriesEpisodes map[string][]playback.SeriesEpisode
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
	// Mirror the postgres adapter: completed is sticky-true on conflict;
	// play_count increments on false→true transition, otherwise takes max
	// so progress ticks that pass PlayCount=0 don't regress the counter.
	k := key(string(wp.ProfileID), string(wp.MediaItemID))
	if prev, ok := r.items[k]; ok {
		switch {
		case wp.Completed && !prev.Completed:
			wp.PlayCount = prev.PlayCount + 1
		case prev.PlayCount > wp.PlayCount:
			wp.PlayCount = prev.PlayCount
		}
		if prev.Completed {
			wp.Completed = true
		}
	}
	r.items[k] = wp
	return nil
}

// UpsertMany mirrors the batch port method: same semantics, entry by entry.
func (r *FakeWatchProgressRepository) UpsertMany(ctx context.Context, entries []*playback.WatchProgress) error {
	for _, wp := range entries {
		if err := r.Upsert(ctx, wp); err != nil {
			return err
		}
	}
	return nil
}

func (r *FakeWatchProgressRepository) Delete(_ context.Context, profileID, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, key(profileID, mediaItemID))
	return nil
}

func (r *FakeWatchProgressRepository) DeleteByProfile(_ context.Context, profileID string) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for k, wp := range r.items {
		if wp != nil && string(wp.ProfileID) == profileID {
			delete(r.items, k)
			n++
		}
	}
	return n, nil
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

func (r *FakeWatchProgressRepository) GetByProfileCursor(_ context.Context, profileID string, cursor *playback.WatchHistoryCursor, limit int) ([]*playback.WatchProgress, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playback.WatchProgress
	for _, wp := range r.items {
		if string(wp.ProfileID) == profileID {
			result = append(result, wp)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].LastPlayedAt.Equal(result[j].LastPlayedAt) {
			return string(result[i].MediaItemID) > string(result[j].MediaItemID)
		}
		return result[i].LastPlayedAt.After(result[j].LastPlayedAt)
	})
	if cursor != nil {
		trimmed := result[:0]
		for _, wp := range result {
			after := wp.LastPlayedAt.Before(cursor.LastPlayedAt) ||
				(wp.LastPlayedAt.Equal(cursor.LastPlayedAt) && string(wp.MediaItemID) < cursor.MediaItemID)
			if after {
				trimmed = append(trimmed, wp)
			}
		}
		result = trimmed
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
		if string(wp.ProfileID) == profileID && !wp.Completed && !wp.Position.IsZero() {
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

func (r *FakeWatchProgressRepository) ListSeriesEpisodes(_ context.Context, _, seriesID string) ([]playback.SeriesEpisode, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.SeriesEpisodes[seriesID], nil
}
