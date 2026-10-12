package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
)

// ErrDuplicateKey mirrors what postgres raises when an upsert lands on an
// existing primary key with no arbiter to resolve it.
var ErrDuplicateKey = errors.New("duplicate key value violates unique constraint")

type FakeCollectionCommandRepo struct {
	mu          sync.RWMutex
	collections map[string]*library.Collection
	items       map[string][]collectionItem
	Err         error
}

type collectionItem struct {
	mediaItemID string
	sortOrder   int
}

func NewFakeCollectionCommandRepo() *FakeCollectionCommandRepo {
	return &FakeCollectionCommandRepo{
		collections: make(map[string]*library.Collection),
		items:       make(map[string][]collectionItem),
	}
}

func (r *FakeCollectionCommandRepo) Create(_ context.Context, col *library.Collection) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collections[col.ID] = col
	return nil
}

func (r *FakeCollectionCommandRepo) Upsert(_ context.Context, col *library.Collection) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if col.TmdbID != nil {
		for _, existing := range r.collections {
			if existing.TmdbID != nil && *existing.TmdbID == *col.TmdbID {
				existing.Name = col.Name
				existing.Overview = col.Overview
				existing.PosterURL = col.PosterURL
				existing.BackdropURL = col.BackdropURL
				existing.UpdatedAt = col.UpdatedAt
				*col = *existing
				return nil
			}
		}
	}
	// The real upsert arbitrates on tmdb_id only. A collection without one that
	// already exists is an INSERT onto its own primary key, which postgres
	// rejects - so renaming a hand-built collection through this path returned
	// 500 while this fake happily overwrote the map (reproduced on dev
	// 2026-08-04). Model the constraint, or the fake keeps hiding it.
	if _, exists := r.collections[col.ID]; exists {
		return ErrDuplicateKey
	}
	r.collections[col.ID] = col
	return nil
}

func (r *FakeCollectionCommandRepo) Update(_ context.Context, col *library.Collection) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.collections[col.ID]; !ok {
		return ErrNotFound
	}
	r.collections[col.ID] = col
	return nil
}

func (r *FakeCollectionCommandRepo) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.collections[id]; !ok {
		return ErrNotFound
	}
	delete(r.collections, id)
	delete(r.items, id)
	return nil
}

func (r *FakeCollectionCommandRepo) LinkItem(_ context.Context, collectionID, mediaItemID string, sortOrder int) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, it := range r.items[collectionID] {
		if it.mediaItemID == mediaItemID {
			return nil
		}
	}
	r.items[collectionID] = append(r.items[collectionID], collectionItem{mediaItemID: mediaItemID, sortOrder: sortOrder})
	return nil
}

func (r *FakeCollectionCommandRepo) UnlinkItem(_ context.Context, collectionID, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]collectionItem, 0, len(r.items[collectionID]))
	for _, it := range r.items[collectionID] {
		if it.mediaItemID != mediaItemID {
			kept = append(kept, it)
		}
	}
	r.items[collectionID] = kept
	return nil
}

func (r *FakeCollectionCommandRepo) SetItemOrder(_ context.Context, collectionID string, mediaItemIDs []string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	pos := make(map[string]int, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		pos[id] = i + 1
	}
	// Postgres semantics, faithfully: an id that is not a member fails the
	// WHOLE reorder (rolled back), it is not silently skipped.
	matched := 0
	for _, it := range r.items[collectionID] {
		if _, ok := pos[it.mediaItemID]; ok {
			matched++
		}
	}
	if matched != len(pos) {
		return library.ErrCollectionReorderDrift
	}
	for i, it := range r.items[collectionID] {
		if p, ok := pos[it.mediaItemID]; ok {
			r.items[collectionID][i].sortOrder = p
		}
	}
	return nil
}
