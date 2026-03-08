package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/metadata"
)

type FakeArtworkRepository struct {
	mu    sync.RWMutex
	items map[string]*metadata.ArtworkRecord
}

func NewFakeArtworkRepository() *FakeArtworkRepository {
	return &FakeArtworkRepository{items: make(map[string]*metadata.ArtworkRecord)}
}

func (r *FakeArtworkRepository) Create(_ context.Context, a *metadata.ArtworkRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[a.ID] = a
	return nil
}

func (r *FakeArtworkRepository) GetByMediaItem(_ context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*metadata.ArtworkRecord
	for _, a := range r.items {
		if a.MediaItemID == mediaItemID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (r *FakeArtworkRepository) GetByPerson(_ context.Context, personID string) ([]*metadata.ArtworkRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*metadata.ArtworkRecord
	for _, a := range r.items {
		if a.PersonID != nil && *a.PersonID == personID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (r *FakeArtworkRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *FakeArtworkRepository) DeleteByMediaItemID(_ context.Context, mediaItemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, a := range r.items {
		if a.MediaItemID == mediaItemID {
			delete(r.items, id)
		}
	}
	return nil
}
