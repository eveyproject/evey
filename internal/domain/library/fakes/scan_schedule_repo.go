package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeScanScheduleRepository struct {
	mu    sync.RWMutex
	items map[string]*library.ScanSchedule
}

func NewFakeScanScheduleRepository() *FakeScanScheduleRepository {
	return &FakeScanScheduleRepository{items: make(map[string]*library.ScanSchedule)}
}

func (r *FakeScanScheduleRepository) Upsert(_ context.Context, s *library.ScanSchedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[s.LibraryID] = s
	return nil
}

func (r *FakeScanScheduleRepository) GetByLibraryID(_ context.Context, libraryID string) (*library.ScanSchedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.items[libraryID]
	if !ok {
		return nil, library.ErrNotFound
	}
	return s, nil
}

func (r *FakeScanScheduleRepository) Delete(_ context.Context, libraryID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, libraryID)
	return nil
}

func (r *FakeScanScheduleRepository) ListEnabled(_ context.Context) ([]*library.ScanSchedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.ScanSchedule
	for _, s := range r.items {
		if s.Enabled {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *FakeScanScheduleRepository) UpdateAfterRun(_ context.Context, libraryID string, lastRun, nextRun time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.items[libraryID]
	if !ok {
		return library.ErrNotFound
	}
	s.LastRunAt = &lastRun
	s.NextRunAt = &nextRun
	return nil
}
