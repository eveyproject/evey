package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeMediaItemQueryRepo struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	cast  map[string][]library.CastMember
}

func NewFakeMediaItemQueryRepo() *FakeMediaItemQueryRepo {
	return &FakeMediaItemQueryRepo{
		items: make(map[string]*library.MediaItem),
		cast:  make(map[string][]library.CastMember),
	}
}

func (r *FakeMediaItemQueryRepo) GetByID(_ context.Context, id string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *FakeMediaItemQueryRepo) List(_ context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.MediaItem
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		if filter.Type != nil && item.Type != *filter.Type {
			continue
		}
		result = append(result, item)
	}
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return nil, nil
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *FakeMediaItemQueryRepo) Count(_ context.Context, filter library.ListMediaItemsFilter) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if filter.Type != nil && item.Type != *filter.Type {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		count++
	}
	return count, nil
}

func (r *FakeMediaItemQueryRepo) GetCastByMediaItem(_ context.Context, mediaItemID string) ([]library.CastMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cast[mediaItemID], nil
}

// Seed adds items for testing.
func (r *FakeMediaItemQueryRepo) Seed(item *library.MediaItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
}

// SeedCast sets cast for a media item.
func (r *FakeMediaItemQueryRepo) SeedCast(mediaItemID string, cast []library.CastMember) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cast[mediaItemID] = cast
}
