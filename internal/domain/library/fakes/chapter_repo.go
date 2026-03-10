package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeChapterRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.Chapter
}

func NewFakeChapterRepository() *FakeChapterRepository {
	return &FakeChapterRepository{items: make(map[string][]*library.Chapter)}
}

func (r *FakeChapterRepository) ReplaceBatch(_ context.Context, mediaItemID string, chapters []*library.Chapter) error {
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[mediaItemID], nil
}
