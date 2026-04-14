package fakes

import (
	"context"
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
		result = append(result, e)
	}
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}
