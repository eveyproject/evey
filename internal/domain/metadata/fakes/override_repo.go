package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/metadata"
)

type FakeMetadataOverrideRepository struct {
	mu    sync.RWMutex
	items map[string]*metadata.MetadataOverride
}

func NewFakeMetadataOverrideRepository() *FakeMetadataOverrideRepository {
	return &FakeMetadataOverrideRepository{items: make(map[string]*metadata.MetadataOverride)}
}

func (r *FakeMetadataOverrideRepository) Set(_ context.Context, o *metadata.MetadataOverride) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[o.ID] = o
	return nil
}

func (r *FakeMetadataOverrideRepository) Get(_ context.Context, id string) (*metadata.MetadataOverride, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return o, nil
}

func (r *FakeMetadataOverrideRepository) GetByMediaItem(_ context.Context, mediaItemID string) ([]*metadata.MetadataOverride, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*metadata.MetadataOverride
	for _, o := range r.items {
		if o.MediaItemID == mediaItemID {
			result = append(result, o)
		}
	}
	return result, nil
}

func (r *FakeMetadataOverrideRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
