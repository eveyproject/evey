package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/subtitle"
)

type FakeRepository struct {
	mu    sync.RWMutex
	items map[string]*subtitle.SubtitleDownload
	Err   error
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{items: make(map[string]*subtitle.SubtitleDownload)}
}

func (r *FakeRepository) Save(_ context.Context, sub *subtitle.SubtitleDownload) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[sub.ID] = sub
	return nil
}

func (r *FakeRepository) GetByMediaItem(_ context.Context, mediaItemID string) ([]*subtitle.SubtitleDownload, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*subtitle.SubtitleDownload
	for _, s := range r.items {
		if s.MediaItemID == mediaItemID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *FakeRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return subtitle.ErrNotFound
	}
	delete(r.items, id)
	return nil
}
