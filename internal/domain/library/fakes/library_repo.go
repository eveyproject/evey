package fakes

import (
	"context"
	"sort"
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
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

func (r *FakeLibraryRepository) List(_ context.Context, filter library.ListLibrariesFilter) ([]*library.Library, error) {
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
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return applyLibrariesFilter(result, filter), nil
}

func applyLibrariesFilter(libs []*library.Library, filter library.ListLibrariesFilter) []*library.Library {
	if filter.Limit <= 0 {
		return libs
	}
	limit := filter.Limit
	if limit > library.MaxListLibrariesLimit {
		limit = library.MaxListLibrariesLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(libs) {
		return []*library.Library{}
	}
	end := offset + limit
	if end > len(libs) {
		end = len(libs)
	}
	return libs[offset:end]
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

var (
	_ library.LibraryCommandRepo = (*FakeLibraryRepository)(nil)
	_ library.LibraryQueryRepo   = (*FakeLibraryRepository)(nil)
)
