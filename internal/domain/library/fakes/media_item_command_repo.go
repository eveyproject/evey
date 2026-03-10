package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeMediaItemCommandRepo struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	GetByFileFingerprintErr  error
	GetByLibraryTypeTitleErr error
}

func NewFakeMediaItemCommandRepo() *FakeMediaItemCommandRepo {
	return &FakeMediaItemCommandRepo{items: make(map[string]*library.MediaItem)}
}

func (r *FakeMediaItemCommandRepo) Create(_ context.Context, item *library.MediaItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
	return nil
}

func (r *FakeMediaItemCommandRepo) Update(_ context.Context, item *library.MediaItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	r.items[item.ID] = item
	return nil
}

func (r *FakeMediaItemCommandRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *FakeMediaItemCommandRepo) GetByFilePath(_ context.Context, path string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.FilePath == path {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemCommandRepo) GetByFileFingerprint(_ context.Context, fingerprint string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.GetByFileFingerprintErr != nil {
		return nil, r.GetByFileFingerprintErr
	}
	for _, item := range r.items {
		if item.FileFingerprint == fingerprint {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemCommandRepo) GetByLibraryTypeTitle(_ context.Context, libraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.GetByLibraryTypeTitleErr != nil {
		return nil, r.GetByLibraryTypeTitleErr
	}
	for _, item := range r.items {
		if item.LibraryID == libraryID && item.Type == itemType && item.Title == title &&
			(parentID == "" || item.ParentID == parentID) {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

// GetByID is a test helper to inspect created items (not part of the command interface).
func (r *FakeMediaItemCommandRepo) GetByID(_ context.Context, id string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

// All returns all items for test assertions.
func (r *FakeMediaItemCommandRepo) All() []*library.MediaItem {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*library.MediaItem, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result
}
