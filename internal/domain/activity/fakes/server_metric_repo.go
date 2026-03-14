package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type FakeServerMetricRepository struct {
	mu    sync.RWMutex
	items []*activity.ServerMetric
	Err   error
}

func NewFakeServerMetricRepository() *FakeServerMetricRepository {
	return &FakeServerMetricRepository{}
}

func (r *FakeServerMetricRepository) Create(_ context.Context, metric *activity.ServerMetric) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, metric)
	return nil
}

func (r *FakeServerMetricRepository) GetByTimeRange(_ context.Context, from, to time.Time) ([]*activity.ServerMetric, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*activity.ServerMetric
	for _, m := range r.items {
		if (m.RecordedAt.Equal(from) || m.RecordedAt.After(from)) && (m.RecordedAt.Equal(to) || m.RecordedAt.Before(to)) {
			result = append(result, m)
		}
	}
	return result, nil
}

func (r *FakeServerMetricRepository) Purge(_ context.Context, before time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var kept []*activity.ServerMetric
	for _, m := range r.items {
		if m.RecordedAt.After(before) || m.RecordedAt.Equal(before) {
			kept = append(kept, m)
		}
	}
	r.items = kept
	return nil
}
