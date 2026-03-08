package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeMediaItemRepository struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
}

func NewFakeMediaItemRepository() *FakeMediaItemRepository {
	return &FakeMediaItemRepository{items: make(map[string]*library.MediaItem)}
}

func (r *FakeMediaItemRepository) Create(_ context.Context, item *library.MediaItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
	return nil
}

func (r *FakeMediaItemRepository) GetByID(_ context.Context, id string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *FakeMediaItemRepository) List(_ context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
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

func (r *FakeMediaItemRepository) Update(_ context.Context, item *library.MediaItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	r.items[item.ID] = item
	return nil
}

func (r *FakeMediaItemRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *FakeMediaItemRepository) GetByFilePath(_ context.Context, path string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.FilePath == path {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemRepository) GetByFileFingerprint(_ context.Context, fingerprint string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.FileFingerprint == fingerprint {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemRepository) Count(_ context.Context, filter library.ListMediaItemsFilter) (int, error) {
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

func (r *FakeMediaItemRepository) GetByLibraryTypeTitle(_ context.Context, libraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.LibraryID == libraryID && item.Type == itemType && item.Title == title &&
			(parentID == "" || item.ParentID == parentID) {
			return item, nil
		}
	}
	return nil, ErrNotFound
}
