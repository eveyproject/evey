package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type FakeAuditLogRepository struct {
	mu    sync.RWMutex
	items []*activity.AuditLogEntry
	Err   error
}

func NewFakeAuditLogRepository() *FakeAuditLogRepository {
	return &FakeAuditLogRepository{}
}

func (r *FakeAuditLogRepository) Create(_ context.Context, entry *activity.AuditLogEntry) error {
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

func (r *FakeAuditLogRepository) List(_ context.Context, filter activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*activity.AuditLogEntry
	for _, e := range r.items {
		if filter.UserID != nil && e.UserID != *filter.UserID {
			continue
		}
		if filter.Action != nil && e.Action != *filter.Action {
			continue
		}
		if filter.ResourceType != nil && e.ResourceType != *filter.ResourceType {
			continue
		}
		result = append(result, e)
	}
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return nil, nil
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}
