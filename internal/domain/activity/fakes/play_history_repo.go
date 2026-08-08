package fakes

import (
	"context"
	"sort"
	"sync"

	"github.com/evey-project/evey/internal/domain/activity"
)

type FakePlayHistoryRepository struct {
	mu    sync.RWMutex
	items []*activity.PlayHistoryEntry
	Err   error
}

func NewFakePlayHistoryRepository() *FakePlayHistoryRepository {
	return &FakePlayHistoryRepository{}
}

func (r *FakePlayHistoryRepository) Add(entries ...*activity.PlayHistoryEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, entries...)
}

func (r *FakePlayHistoryRepository) List(_ context.Context, filter activity.PlayHistoryFilter) ([]*activity.PlayHistoryEntry, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*activity.PlayHistoryEntry
	for _, e := range r.items {
		if filter.ProfileID != "" && e.ProfileID != filter.ProfileID {
			continue
		}
		if filter.MediaItemID != "" && e.MediaItemID != filter.MediaItemID {
			continue
		}
		if filter.From != nil && e.StartedAt.Before(*filter.From) {
			continue
		}
		if filter.To != nil && e.StartedAt.After(*filter.To) {
			continue
		}
		result = append(result, e)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	if filter.Cursor != nil {
		trimmed := result[:0]
		for _, e := range result {
			after := e.StartedAt.Before(filter.Cursor.StartedAt) ||
				(e.StartedAt.Equal(filter.Cursor.StartedAt) && e.ID < filter.Cursor.SessionID)
			if after {
				trimmed = append(trimmed, e)
			}
		}
		result = trimmed
	} else if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	}
	// Postgres clamps a zero Limit to 500 rather than returning everything
	// (see the matching repository under adapters/driven/postgres). A fake that
	// hands back the whole set makes an unpaginated read look correct in tests
	// and silently truncate in production - the shape of bug this repository
	// has already shipped once.
	const defaultListLimit = 500 // mirrors the postgres repository's clamp
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *FakePlayHistoryRepository) Count(_ context.Context, filter activity.PlayHistoryFilter) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, e := range r.items {
		if filter.ProfileID != "" && e.ProfileID != filter.ProfileID {
			continue
		}
		if filter.MediaItemID != "" && e.MediaItemID != filter.MediaItemID {
			continue
		}
		if filter.From != nil && e.StartedAt.Before(*filter.From) {
			continue
		}
		if filter.To != nil && e.StartedAt.After(*filter.To) {
			continue
		}
		n++
	}
	return n, nil
}
