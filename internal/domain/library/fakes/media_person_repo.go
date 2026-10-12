package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeMediaPersonRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.MediaPerson
	Err   error

	// DeleteByMediaItemIDCalls / DeleteByMediaItemIDsCalls count invocations
	// of the single-item vs batch delete, so tests can assert the N+1 is gone.
	DeleteByMediaItemIDCalls  int
	DeleteByMediaItemIDsCalls int
	// CallLog, when non-nil, records a tag ("persons") on every delete call.
	// Shared across the three repos used by ResetItemMetadata so a test can
	// assert the overrides-before-artwork-before-persons ordering invariant.
	CallLog *[]string
}

func NewFakeMediaPersonRepository() *FakeMediaPersonRepository {
	return &FakeMediaPersonRepository{items: make(map[string][]*library.MediaPerson)}
}

func (r *FakeMediaPersonRepository) Create(_ context.Context, mp *library.MediaPerson) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(mp.MediaItemID)] = append(r.items[string(mp.MediaItemID)], mp)
	return nil
}

func (r *FakeMediaPersonRepository) CreateBatch(_ context.Context, mps []*library.MediaPerson) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, mp := range mps {
		r.items[string(mp.MediaItemID)] = append(r.items[string(mp.MediaItemID)], mp)
	}
	return nil
}

func (r *FakeMediaPersonRepository) DeleteByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.DeleteByMediaItemIDCalls++
	if r.CallLog != nil {
		*r.CallLog = append(*r.CallLog, "persons")
	}
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, string(mediaItemID))
	return nil
}

// DeleteByMediaItemIDs is the batch form: one map-delete pass instead of one
// call per item.
func (r *FakeMediaPersonRepository) DeleteByMediaItemIDs(_ context.Context, mediaItemIDs []shared.MediaItemID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.DeleteByMediaItemIDsCalls++
	if r.CallLog != nil {
		*r.CallLog = append(*r.CallLog, "persons")
	}
	if r.Err != nil {
		return r.Err
	}
	for _, id := range mediaItemIDs {
		delete(r.items, string(id))
	}
	return nil
}

func (r *FakeMediaPersonRepository) GetByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaPerson, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return r.items[string(mediaItemID)], nil
}
