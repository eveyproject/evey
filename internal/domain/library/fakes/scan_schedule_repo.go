package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakeScanScheduleRepository struct {
	mu    sync.RWMutex
	items map[string]*library.ScanSchedule
	Err   error
}

func NewFakeScanScheduleRepository() *FakeScanScheduleRepository {
	return &FakeScanScheduleRepository{items: make(map[string]*library.ScanSchedule)}
}

func (r *FakeScanScheduleRepository) Upsert(_ context.Context, s *library.ScanSchedule) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(s.LibraryID)] = s
	return nil
}

func (r *FakeScanScheduleRepository) GetByLibraryID(_ context.Context, libraryID string) (*library.ScanSchedule, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	s, ok := r.items[libraryID]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

func (r *FakeScanScheduleRepository) Delete(_ context.Context, libraryID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, libraryID)
	return nil
}

func (r *FakeScanScheduleRepository) List(_ context.Context) ([]*library.ScanSchedule, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.ScanSchedule
	for _, s := range r.items {
		result = append(result, s)
	}
	return result, nil
}

func (r *FakeScanScheduleRepository) ListEnabled(_ context.Context) ([]*library.ScanSchedule, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*library.ScanSchedule
	for _, s := range r.items {
		if s.Enabled {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *FakeScanScheduleRepository) UpdateAfterRun(_ context.Context, libraryID string, lastRun, nextRun time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	s, ok := r.items[libraryID]
	if !ok {
		return ErrNotFound
	}
	s.LastRunAt = &lastRun
	s.NextRunAt = &nextRun
	return nil
}
