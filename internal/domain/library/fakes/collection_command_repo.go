package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeCollectionCommandRepo struct {
	mu          sync.RWMutex
	collections map[string]*library.Collection
	items       map[string][]string
}

func NewFakeCollectionCommandRepo() *FakeCollectionCommandRepo {
	return &FakeCollectionCommandRepo{
		collections: make(map[string]*library.Collection),
		items:       make(map[string][]string),
	}
}

func (r *FakeCollectionCommandRepo) Create(_ context.Context, col *library.Collection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collections[col.ID] = col
	return nil
}

func (r *FakeCollectionCommandRepo) Update(_ context.Context, col *library.Collection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.collections[col.ID]; !ok {
		return ErrNotFound
	}
	r.collections[col.ID] = col
	return nil
}

func (r *FakeCollectionCommandRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.collections[id]; !ok {
		return ErrNotFound
	}
	delete(r.collections, id)
	delete(r.items, id)
	return nil
}

func (r *FakeCollectionCommandRepo) AddItem(_ context.Context, collectionID, mediaItemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[collectionID] = append(r.items[collectionID], mediaItemID)
	return nil
}

func (r *FakeCollectionCommandRepo) RemoveItem(_ context.Context, collectionID, mediaItemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.items[collectionID]
	for i, id := range ids {
		if id == mediaItemID {
			r.items[collectionID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}
