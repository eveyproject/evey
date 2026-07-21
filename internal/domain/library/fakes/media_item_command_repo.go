package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeMediaItemCommandRepo struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	// UpdatedIDs records every Update call, in order. Needed because Create
	// stores the CALLER'S pointer: a service that mutates the item in memory
	// is visible through items[] whether or not it ever persisted, so
	// asserting on the stored struct cannot prove a write happened.
	UpdatedIDs []string
	// Per-method error injection; zero value = no error (backward compatible)
	ErrCreate                error
	ErrUpdate                error
	ErrDelete                error
	GetByFileFingerprintErr  error
	GetByLibraryTypeTitleErr error
	ListByLibraryIDErr       error
}

func NewFakeMediaItemCommandRepo() *FakeMediaItemCommandRepo {
	return &FakeMediaItemCommandRepo{items: make(map[string]*library.MediaItem)}
}

func (r *FakeMediaItemCommandRepo) Create(_ context.Context, item *library.MediaItem) error {
	if r.ErrCreate != nil {
		return r.ErrCreate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := *item
	r.items[string(item.ID)] = &stored
	return nil
}

func (r *FakeMediaItemCommandRepo) Update(_ context.Context, item *library.MediaItem) error {
	if r.ErrUpdate != nil {
		return r.ErrUpdate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[string(item.ID)]; !ok {
		return ErrNotFound
	}
	stored := *item
	r.items[string(item.ID)] = &stored
	r.UpdatedIDs = append(r.UpdatedIDs, string(item.ID))
	return nil
}

func (r *FakeMediaItemCommandRepo) Delete(_ context.Context, id shared.MediaItemID) error {
	if r.ErrDelete != nil {
		return r.ErrDelete
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[string(id)]; !ok {
		return ErrNotFound
	}
	delete(r.items, string(id))
	return nil
}

func (r *FakeMediaItemCommandRepo) DeleteByIDs(_ context.Context, ids []string) error {
	if r.ErrDelete != nil {
		return r.ErrDelete
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		delete(r.items, id)
	}
	return nil
}

func (r *FakeMediaItemCommandRepo) GetByFilePath(_ context.Context, path string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.File != nil && item.File.Path == path {
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
		if item.File != nil && item.File.Fingerprint == fingerprint {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemCommandRepo) GetByLibraryTypeTitle(_ context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.GetByLibraryTypeTitleErr != nil {
		return nil, r.GetByLibraryTypeTitleErr
	}
	for _, item := range r.items {
		if item.LibraryID == libraryID && string(item.Type) == itemType && item.Title == title &&
			(parentID == "" || item.ParentID == parentID) {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

// GetByID satisfies MediaItemCommandRepo and is also used as a test helper.
func (r *FakeMediaItemCommandRepo) GetByID(_ context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *FakeMediaItemCommandRepo) ListFilePaths(_ context.Context, libraryID shared.LibraryID) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]string)
	for _, item := range r.items {
		if item.LibraryID == libraryID && item.File != nil && item.File.Path != "" {
			result[string(item.ID)] = item.File.Path
		}
	}
	return result, nil
}

func (r *FakeMediaItemCommandRepo) ListByLibraryID(_ context.Context, libraryID shared.LibraryID) ([]*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.ListByLibraryIDErr != nil {
		return nil, r.ListByLibraryIDErr
	}
	var result []*library.MediaItem
	for _, item := range r.items {
		if item.LibraryID == libraryID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *FakeMediaItemCommandRepo) ListScanSummaries(_ context.Context, libraryID shared.LibraryID) ([]library.MediaItemScanRow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []library.MediaItemScanRow
	for _, item := range r.items {
		if item.LibraryID == libraryID {
			row := library.MediaItemScanRow{ID: string(item.ID)}
			if item.File != nil {
				row.FilePath = item.File.Path
				row.FileFingerprint = item.File.Fingerprint
				row.FileSize = item.File.Size
				row.ModTime = item.File.ModTime
				row.StreamProbeAttempts = item.File.StreamProbeAttempts
			}
			result = append(result, row)
		}
	}
	return result, nil
}

func (r *FakeMediaItemCommandRepo) ListContainerInfo(_ context.Context, libraryID shared.LibraryID) ([]library.MediaItemContainerRow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []library.MediaItemContainerRow
	for _, item := range r.items {
		if item.LibraryID == libraryID {
			result = append(result, library.MediaItemContainerRow{
				ID:       string(item.ID),
				ParentID: item.ParentID,
				Type:     string(item.Type),
			})
		}
	}
	return result, nil
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
