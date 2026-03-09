package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeLibraryQueryRepo struct {
	mu    sync.RWMutex
	items map[string]*library.Library
}

func NewFakeLibraryQueryRepo() *FakeLibraryQueryRepo {
	return &FakeLibraryQueryRepo{items: make(map[string]*library.Library)}
}

func (r *FakeLibraryQueryRepo) GetByID(_ context.Context, id string) (*library.Library, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lib, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return lib, nil
}

func (r *FakeLibraryQueryRepo) List(_ context.Context) ([]*library.Library, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*library.Library, 0, len(r.items))
	for _, lib := range r.items {
		result = append(result, lib)
	}
	return result, nil
}

// Seed adds a library for testing (write-side helper, not part of the interface).
func (r *FakeLibraryQueryRepo) Seed(lib *library.Library) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[lib.ID] = lib
}
