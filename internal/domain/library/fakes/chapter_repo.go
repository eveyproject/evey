package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeChapterRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.Chapter
	Err   error
}

func NewFakeChapterRepository() *FakeChapterRepository {
	return &FakeChapterRepository{items: make(map[string][]*library.Chapter)}
}

func (r *FakeChapterRepository) ReplaceBatch(_ context.Context, mediaItemID string, chapters []*library.Chapter) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(chapters) == 0 {
		delete(r.items, mediaItemID)
		return nil
	}
	r.items[mediaItemID] = chapters
	return nil
}

func (r *FakeChapterRepository) GetByMediaItemID(_ context.Context, mediaItemID string) ([]*library.Chapter, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[mediaItemID], nil
}

func (r *FakeChapterRepository) All() []*library.Chapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.Chapter
	for _, chs := range r.items {
		result = append(result, chs...)
	}
	return result
}
