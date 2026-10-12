package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

type FakePluginRuntimeRepository struct {
	mu    sync.RWMutex
	items map[string]*plugin.PluginRuntime
	Err   error
}

func NewFakePluginRuntimeRepository() *FakePluginRuntimeRepository {
	return &FakePluginRuntimeRepository{items: make(map[string]*plugin.PluginRuntime)}
}

func (r *FakePluginRuntimeRepository) Create(_ context.Context, rt *plugin.PluginRuntime) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[rt.ID] = rt
	return nil
}

func (r *FakePluginRuntimeRepository) GetByLanguageAndVersion(_ context.Context, language, version string) (*plugin.PluginRuntime, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rt := range r.items {
		if rt.Language == language && rt.Version == version {
			return rt, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakePluginRuntimeRepository) ListByLanguage(_ context.Context, language string) ([]*plugin.PluginRuntime, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*plugin.PluginRuntime
	for _, rt := range r.items {
		if rt.Language == language {
			result = append(result, rt)
		}
	}
	return result, nil
}

func (r *FakePluginRuntimeRepository) List(_ context.Context) ([]*plugin.PluginRuntime, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*plugin.PluginRuntime, 0, len(r.items))
	for _, rt := range r.items {
		result = append(result, rt)
	}
	return result, nil
}

func (r *FakePluginRuntimeRepository) Delete(_ context.Context, id string) error {
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
