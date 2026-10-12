package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeChapterRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.Chapter
	Err   error
}

func NewFakeChapterRepository() *FakeChapterRepository {
	return &FakeChapterRepository{items: make(map[string][]*library.Chapter)}
}

func (r *FakeChapterRepository) ReplaceBatch(_ context.Context, mediaItemID shared.MediaItemID, chapters []*library.Chapter) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if len(chapters) == 0 {
		delete(r.items, string(mediaItemID))
		return nil
	}
	r.items[string(mediaItemID)] = chapters
	return nil
}

func (r *FakeChapterRepository) GetByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) ([]*library.Chapter, error) {
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

func (r *FakeChapterRepository) All() []*library.Chapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.Chapter
	for _, chs := range r.items {
		result = append(result, chs...)
	}
	return result
}
