package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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
	if r.Err != nil {
		return r.Err
	}
	for _, s := range streams {
		r.items[string(s.MediaItemID)] = append(r.items[string(s.MediaItemID)], s)
	}
	return nil
}

func (r *FakeMediaStreamRepository) ReplaceBatch(_ context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, string(mediaItemID))
	for _, s := range streams {
		r.items[string(s.MediaItemID)] = append(r.items[string(s.MediaItemID)], s)
	}
	return nil
}

func (r *FakeMediaStreamRepository) GetByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaStream, error) {
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

func (r *FakeMediaStreamRepository) GetByMediaItemIDs(_ context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]*library.MediaStream, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[shared.MediaItemID][]*library.MediaStream, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if streams, ok := r.items[string(id)]; ok {
			result[id] = streams
		}
	}
	return result, nil
}

func (r *FakeMediaStreamRepository) DeleteByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) error {
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
