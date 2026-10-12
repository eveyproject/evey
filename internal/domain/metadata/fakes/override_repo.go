package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/metadata"
)

type FakeMetadataOverrideRepository struct {
	mu    sync.RWMutex
	items map[string]*metadata.MetadataOverride
	Err   error

	// DeleteByMediaItemIDCalls / DeleteByMediaItemIDsCalls count invocations
	// of the single-item vs batch delete, so tests can assert the N+1 is gone.
	DeleteByMediaItemIDCalls  int
	DeleteByMediaItemIDsCalls int
	// CallLog, when non-nil, records a tag ("overrides") on every delete call.
	// Shared across the three fakes used by ResetItemMetadata so a test can
	// assert the overrides-before-artwork-before-persons ordering invariant.
	CallLog *[]string
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
		if string(o.MediaItemID) == mediaItemID {
			result = append(result, o)
		}
	}
	return result, nil
}

func (r *FakeMetadataOverrideRepository) GetByMediaItems(_ context.Context, mediaItemIDs []string) (map[string][]*metadata.MetadataOverride, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	wanted := make(map[string]bool, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		wanted[id] = true
	}
	result := make(map[string][]*metadata.MetadataOverride)
	for _, o := range r.items {
		if wanted[string(o.MediaItemID)] {
			result[string(o.MediaItemID)] = append(result[string(o.MediaItemID)], o)
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
		if wanted[string(o.MediaItemID)] {
			result[string(o.MediaItemID)] = true
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
	r.DeleteByMediaItemIDCalls++
	if r.CallLog != nil {
		*r.CallLog = append(*r.CallLog, "overrides")
	}
	if r.Err != nil {
		return r.Err
	}
	for id, o := range r.items {
		if string(o.MediaItemID) == mediaItemID {
			delete(r.items, id)
		}
	}
	return nil
}

// DeleteByMediaItemIDs is the batch form: one pass over the map instead of
// one call (and one map scan) per item.
func (r *FakeMetadataOverrideRepository) DeleteByMediaItemIDs(_ context.Context, mediaItemIDs []string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.DeleteByMediaItemIDsCalls++
	if r.CallLog != nil {
		*r.CallLog = append(*r.CallLog, "overrides")
	}
	if r.Err != nil {
		return r.Err
	}
	wanted := make(map[string]bool, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		wanted[id] = true
	}
	for id, o := range r.items {
		if wanted[string(o.MediaItemID)] {
			delete(r.items, id)
		}
	}
	return nil
}
