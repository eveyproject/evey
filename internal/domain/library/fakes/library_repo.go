package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

var ErrNotFound = shared.ErrNotFound

type FakeLibraryRepository struct {
	mu    sync.RWMutex
	items map[string]*library.Library
	Err   error
}

func NewFakeLibraryRepository() *FakeLibraryRepository {
	return &FakeLibraryRepository{items: make(map[string]*library.Library)}
}

func (r *FakeLibraryRepository) Create(_ context.Context, lib *library.Library) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(lib.ID)] = lib
	return nil
}

func (r *FakeLibraryRepository) GetByID(_ context.Context, id shared.LibraryID) (*library.Library, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	lib, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return lib, nil
}

func (r *FakeLibraryRepository) List(_ context.Context) ([]*library.Library, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make([]*library.Library, 0, len(r.items))
	for _, lib := range r.items {
		result = append(result, lib)
	}
	return result, nil
}

func (r *FakeLibraryRepository) Update(_ context.Context, lib *library.Library) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[string(lib.ID)]; !ok {
		return ErrNotFound
	}
	r.items[string(lib.ID)] = lib
	return nil
}

func (r *FakeLibraryRepository) Delete(_ context.Context, id shared.LibraryID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[string(id)]; !ok {
		return ErrNotFound
	}
	delete(r.items, string(id))
	return nil
}
