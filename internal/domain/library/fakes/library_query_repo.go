package fakes

import (
	"context"
	"sort"
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeLibraryQueryRepo struct {
	mu    sync.RWMutex
	items map[string]*library.Library
	Err   error
}

func NewFakeLibraryQueryRepo() *FakeLibraryQueryRepo {
	return &FakeLibraryQueryRepo{items: make(map[string]*library.Library)}
}

func (r *FakeLibraryQueryRepo) GetByID(_ context.Context, id shared.LibraryID) (*library.Library, error) {
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
	return copyLibrary(lib), nil
}

func (r *FakeLibraryQueryRepo) List(_ context.Context, filter library.ListLibrariesFilter) ([]*library.Library, error) {
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
		result = append(result, copyLibrary(lib))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return applyLibrariesFilter(result, filter), nil
}

// copyLibrary is what a database read or write gives: a library of its own,
// so a caller that edits it (an update) never writes under one still reading
// it (a scan started by the create), and the command fake's in-place edits
// never reach this store behind its lock.
func copyLibrary(lib *library.Library) *library.Library {
	c := *lib
	return &c
}

// Seed adds a library for testing (write-side helper, not part of the interface).
func (r *FakeLibraryQueryRepo) Seed(lib *library.Library) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[string(lib.ID)] = copyLibrary(lib)
}
