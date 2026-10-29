package fakes

import (
	"context"
	"maps"
	"sync"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
)

var ErrNotFound = shared.ErrNotFound

type FakePluginRepository struct {
	mu    sync.RWMutex
	items map[string]*plugin.Plugin
	Err   error
	// CreateErr fails only Create, leaving lookups (GetByName/GetByID) working -
	// used to exercise the "DB write fails after extraction" cleanup path without
	// short-circuiting the earlier existing-plugin lookup.
	CreateErr error
	// UpdateErr fails only Update, for the "files swapped, row not written" path.
	UpdateErr error
}

func NewFakePluginRepository() *FakePluginRepository {
	return &FakePluginRepository{items: make(map[string]*plugin.Plugin)}
}

func (r *FakePluginRepository) Create(_ context.Context, p *plugin.Plugin) error {
	if r.Err != nil {
		return r.Err
	}
	if r.CreateErr != nil {
		return r.CreateErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[p.ID] = p
	return nil
}

func (r *FakePluginRepository) GetByID(_ context.Context, id string) (*plugin.Plugin, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakePluginRepository) GetByName(_ context.Context, name string) (*plugin.Plugin, error) {
	if r.Err != nil {
		return nil, r.Err
	}
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
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*plugin.Plugin, 0, len(r.items))
	for _, p := range r.items {
		result = append(result, p)
	}
	return result, nil
}

func (r *FakePluginRepository) Update(_ context.Context, p *plugin.Plugin) error {
	if r.Err != nil {
		return r.Err
	}
	if r.UpdateErr != nil {
		return r.UpdateErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[p.ID]; !ok {
		return ErrNotFound
	}
	r.items[p.ID] = p
	return nil
}

// UpdateConfig writes ONLY the config field, exactly like the SQL it stands in
// for. Making the fake take the whole struct here would hide the very bug the
// narrow write exists to prevent: a test would pass while the real UPDATE
// replayed permissions and enabled.
func (r *FakePluginRepository) UpdateConfig(_ context.Context, id string, config map[string]any) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	cp := make(map[string]any, len(config))
	maps.Copy(cp, config)
	p.Config = cp
	return nil
}

func (r *FakePluginRepository) Delete(_ context.Context, id string) error {
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
