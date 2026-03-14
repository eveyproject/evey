package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

var ErrNotFound = errors.New("not found")

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

func (r *FakeLoginHistoryRepository) GetByUserID(_ context.Context, userID string, limit, offset int) ([]*activity.LoginEntry, error) {
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

