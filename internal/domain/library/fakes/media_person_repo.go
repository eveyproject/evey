package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeMediaPersonRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.MediaPerson
	Err   error
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
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, string(mediaItemID))
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
