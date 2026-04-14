package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/metadata"
)

type FakeMetadataOverrideRepository struct {
	mu    sync.RWMutex
	items map[string]*metadata.MetadataOverride
	Err   error
}

func NewFakeMetadataOverrideRepository() *FakeMetadataOverrideRepository {
	return &FakeMetadataOverrideRepository{items: make(map[string]*metadata.MetadataOverride)}
}

func (r *FakeMetadataOverrideRepository) Set(_ context.Context, o *metadata.MetadataOverride) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[o.ID] = o
	return nil
}

func (r *FakeMetadataOverrideRepository) Get(_ context.Context, id string) (*metadata.MetadataOverride, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	o, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return o, nil
}

func (r *FakeMetadataOverrideRepository) GetByMediaItem(_ context.Context, mediaItemID string) ([]*metadata.MetadataOverride, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*metadata.MetadataOverride
	for _, o := range r.items {
		if o.MediaItemID == mediaItemID {
			result = append(result, o)
		}
	}
	return result, nil
}

func (r *FakeMetadataOverrideRepository) ListOverriddenMediaItemIDs(_ context.Context, mediaItemIDs []string) (map[string]bool, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	wanted := make(map[string]bool, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		wanted[id] = true
	}
	result := make(map[string]bool)
	for _, o := range r.items {
		if wanted[o.MediaItemID] {
			result[o.MediaItemID] = true
		}
	}
	return result, nil
}

func (r *FakeMetadataOverrideRepository) Delete(_ context.Context, id string) error {
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

func (r *FakeMetadataOverrideRepository) DeleteByMediaItemID(_ context.Context, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	for id, o := range r.items {
		if o.MediaItemID == mediaItemID {
			delete(r.items, id)
		}
	}
	return nil
}
