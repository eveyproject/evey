package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeLibraryCommandRepo struct {
	// OnCreate, when set, sees every library Create stores: a test can mirror
	// it into the query fake, as one database would.
	OnCreate func(lib *library.Library)
	mu       sync.RWMutex
	items    map[string]*library.Library
	Err      error
}

func NewFakeLibraryCommandRepo() *FakeLibraryCommandRepo {
	return &FakeLibraryCommandRepo{items: make(map[string]*library.Library)}
}

func (r *FakeLibraryCommandRepo) Seed(lib *library.Library) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[string(lib.ID)] = lib
}

func (r *FakeLibraryCommandRepo) Create(_ context.Context, lib *library.Library) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(lib.ID)] = lib
	if r.OnCreate != nil {
		r.OnCreate(lib)
	}
	return nil
}

func (r *FakeLibraryCommandRepo) Update(_ context.Context, lib *library.Library) error {
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

func (r *FakeLibraryCommandRepo) TouchLastScannedAt(_ context.Context, id shared.LibraryID, at time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	lib, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	lib.LastScannedAt = &at
	return nil
}

func (r *FakeLibraryCommandRepo) Delete(_ context.Context, id shared.LibraryID) error {
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

// Get returns the stored library for a test's assertions, nil when absent.
func (r *FakeLibraryCommandRepo) Get(id shared.LibraryID) *library.Library {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[string(id)]
}
