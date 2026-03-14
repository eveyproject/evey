package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeLibraryCommandRepo struct {
	mu    sync.RWMutex
	items map[string]*library.Library
	Err   error
}

func NewFakeLibraryCommandRepo() *FakeLibraryCommandRepo {
	return &FakeLibraryCommandRepo{items: make(map[string]*library.Library)}
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
	r.items[lib.ID] = lib
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
	if _, ok := r.items[lib.ID]; !ok {
		return ErrNotFound
	}
	r.items[lib.ID] = lib
	return nil
}

func (r *FakeLibraryCommandRepo) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
