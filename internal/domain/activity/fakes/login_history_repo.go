package fakes

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
)

var ErrNotFound = shared.ErrNotFound

type FakeLoginHistoryRepository struct {
	mu    sync.RWMutex
	items []*activity.LoginEntry
	Err   error
}

func NewFakeLoginHistoryRepository() *FakeLoginHistoryRepository {
	return &FakeLoginHistoryRepository{}
}

func (r *FakeLoginHistoryRepository) Create(_ context.Context, entry *activity.LoginEntry) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items = append(r.items, entry)
	return nil
}

func (r *FakeLoginHistoryRepository) GetByUserID(_ context.Context, userID shared.UserID, limit, offset int) ([]*activity.LoginEntry, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*activity.LoginEntry
	for _, e := range r.items {
		if e.UserID == userID {
			result = append(result, e)
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

func (r *FakeLoginHistoryRepository) ListByUserIDCursor(_ context.Context, userID shared.UserID, successOnly bool, from, to *time.Time, cursor *activity.LoginHistoryCursor, limit int) ([]*activity.LoginEntry, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*activity.LoginEntry
	for _, e := range r.items {
		if e.UserID != userID {
			continue
		}
		if successOnly && !e.Success {
			continue
		}
		if from != nil && e.CreatedAt.Before(*from) {
			continue
		}
		if to != nil && e.CreatedAt.After(*to) {
			continue
		}
		result = append(result, e)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if cursor != nil {
		for i, e := range result {
			after := e.CreatedAt.Before(cursor.StartedAt) ||
				(e.CreatedAt.Equal(cursor.StartedAt) && e.ID < cursor.EntryID)
			if after {
				result = result[i:]
				break
			}
			if i == len(result)-1 {
				result = nil
			}
		}
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}
