package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
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
		if filter.UserID != nil && string(e.UserID) != *filter.UserID {
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

func (r *FakeAuditLogRepository) Count(_ context.Context, filter activity.AuditLogFilter) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, e := range r.items {
		if filter.UserID != nil && string(e.UserID) != *filter.UserID {
			continue
		}
		if filter.Action != nil && e.Action != *filter.Action {
			continue
		}
		if filter.ResourceType != nil && e.ResourceType != *filter.ResourceType {
			continue
		}
		n++
	}
	return n, nil
}

func (r *FakeAuditLogRepository) Purge(_ context.Context, before time.Time) (int64, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]*activity.AuditLogEntry, 0, len(r.items))
	var removed int64
	for _, e := range r.items {
		if e.CreatedAt.Before(before) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	r.items = kept
	return removed, nil
}
