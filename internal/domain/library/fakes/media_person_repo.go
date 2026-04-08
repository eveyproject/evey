package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
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
	r.items[mp.MediaItemID] = append(r.items[mp.MediaItemID], mp)
	return nil
}

func (r *FakeMediaPersonRepository) CreateBatch(_ context.Context, mps []*library.MediaPerson) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, mp := range mps {
		r.items[mp.MediaItemID] = append(r.items[mp.MediaItemID], mp)
	}
	return nil
}

func (r *FakeMediaPersonRepository) DeleteByMediaItemID(_ context.Context, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, mediaItemID)
	return nil
}

func (r *FakeMediaPersonRepository) GetByMediaItemID(_ context.Context, mediaItemID string) ([]*library.MediaPerson, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return r.items[mediaItemID], nil
}
