package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeMediaPersonRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.MediaPerson
}

func NewFakeMediaPersonRepository() *FakeMediaPersonRepository {
	return &FakeMediaPersonRepository{items: make(map[string][]*library.MediaPerson)}
}

func (r *FakeMediaPersonRepository) Create(_ context.Context, mp *library.MediaPerson) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[mp.MediaItemID] = append(r.items[mp.MediaItemID], mp)
	return nil
}

func (r *FakeMediaPersonRepository) DeleteByMediaItemID(_ context.Context, mediaItemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, mediaItemID)
	return nil
}

func (r *FakeMediaPersonRepository) GetByMediaItemID(_ context.Context, mediaItemID string) ([]*library.MediaPerson, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[mediaItemID], nil
}
