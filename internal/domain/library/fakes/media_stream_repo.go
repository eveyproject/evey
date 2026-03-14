package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeMediaStreamRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.MediaStream
	Err   error
}

func NewFakeMediaStreamRepository() *FakeMediaStreamRepository {
	return &FakeMediaStreamRepository{items: make(map[string][]*library.MediaStream)}
}

func (r *FakeMediaStreamRepository) CreateBatch(_ context.Context, streams []*library.MediaStream) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range streams {
		r.items[s.MediaItemID] = append(r.items[s.MediaItemID], s)
	}
	return nil
}

func (r *FakeMediaStreamRepository) GetByMediaItemID(_ context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[mediaItemID], nil
}

func (r *FakeMediaStreamRepository) DeleteByMediaItemID(_ context.Context, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, mediaItemID)
	return nil
}
