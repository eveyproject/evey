package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

var ErrNotFound = errors.New("not found")

type FakePluginRepository struct {
	mu    sync.RWMutex
	items map[string]*plugin.Plugin
}

func NewFakePluginRepository() *FakePluginRepository {
	return &FakePluginRepository{items: make(map[string]*plugin.Plugin)}
}

func (r *FakePluginRepository) Create(_ context.Context, p *plugin.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[p.ID] = p
	return nil
}

func (r *FakePluginRepository) GetByID(_ context.Context, id string) (*plugin.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakePluginRepository) GetByName(_ context.Context, name string) (*plugin.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.items {
		if p.Name == name {
			return p, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakePluginRepository) List(_ context.Context) ([]*plugin.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*plugin.Plugin, 0, len(r.items))
	for _, p := range r.items {
		result = append(result, p)
	}
	return result, nil
}

func (r *FakePluginRepository) Update(_ context.Context, p *plugin.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[p.ID]; !ok {
		return ErrNotFound
	}
	r.items[p.ID] = p
	return nil
}

func (r *FakePluginRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
