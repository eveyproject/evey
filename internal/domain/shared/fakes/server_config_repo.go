package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeServerConfigRepository struct {
	mu    sync.RWMutex
	items map[string]*shared.ServerConfigEntry
	Err   error
}

func NewFakeServerConfigRepository() *FakeServerConfigRepository {
	return &FakeServerConfigRepository{items: make(map[string]*shared.ServerConfigEntry)}
}

func (r *FakeServerConfigRepository) Get(_ context.Context, key string) (*shared.ServerConfigEntry, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.items[key]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return e, nil
}

func (r *FakeServerConfigRepository) Set(_ context.Context, key, value string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[key] = &shared.ServerConfigEntry{Key: key, Value: value, UpdatedAt: time.Now()}
	return nil
}

func (r *FakeServerConfigRepository) Delete(_ context.Context, key string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, key)
	return nil
}

func (r *FakeServerConfigRepository) List(_ context.Context) ([]*shared.ServerConfigEntry, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*shared.ServerConfigEntry, 0, len(r.items))
	for _, e := range r.items {
		result = append(result, e)
	}
	return result, nil
}
