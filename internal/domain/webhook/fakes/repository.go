package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/webhook"
)

var ErrNotFound = shared.ErrNotFound

type FakeRepository struct {
	mu    sync.RWMutex
	items map[string]*webhook.Config
	Err   error
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{items: make(map[string]*webhook.Config)}
}

func (r *FakeRepository) Create(_ context.Context, cfg *webhook.Config) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[cfg.ID] = cfg
	return nil
}

func (r *FakeRepository) GetByID(_ context.Context, id string) (*webhook.Config, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cfg, nil
}

func (r *FakeRepository) List(_ context.Context) ([]*webhook.Config, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*webhook.Config, 0, len(r.items))
	for _, cfg := range r.items {
		result = append(result, cfg)
	}
	return result, nil
}

func (r *FakeRepository) Update(_ context.Context, cfg *webhook.Config) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[cfg.ID]; !ok {
		return ErrNotFound
	}
	r.items[cfg.ID] = cfg
	return nil
}

func (r *FakeRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
