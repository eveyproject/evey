package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

var ErrNotFound = library.ErrNotFound

type FakeLibraryRepository struct {
	mu      sync.RWMutex
	items   map[string]*library.Library
}

func NewFakeLibraryRepository() *FakeLibraryRepository {
	return &FakeLibraryRepository{items: make(map[string]*library.Library)}
}

func (r *FakeLibraryRepository) Create(_ context.Context, lib *library.Library) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[lib.ID] = lib
	return nil
}

func (r *FakeLibraryRepository) GetByID(_ context.Context, id string) (*library.Library, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lib, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return lib, nil
}

func (r *FakeLibraryRepository) List(_ context.Context) ([]*library.Library, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*library.Library, 0, len(r.items))
	for _, lib := range r.items {
		result = append(result, lib)
	}
	return result, nil
}

func (r *FakeLibraryRepository) Update(_ context.Context, lib *library.Library) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[lib.ID]; !ok {
		return ErrNotFound
	}
	r.items[lib.ID] = lib
	return nil
}

func (r *FakeLibraryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
