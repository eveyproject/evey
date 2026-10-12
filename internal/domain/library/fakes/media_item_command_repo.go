package fakes

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeMediaItemCommandRepo struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	// UpdatedIDs records every Update call, in order. Originally a workaround
	// for Create storing the CALLER'S pointer, which made an in-memory mutation
	// indistinguishable from a persisted one. That aliasing is gone (the fake
	// clones on store AND on read), so asserting on the stored struct is now
	// meaningful - but asserting on the CALL stays useful when a test cares that
	// a write happened at all rather than what it wrote.
	UpdatedIDs []string
	// GetByFilePathCalls counts per-file lookups, so a test can assert a scan
	// pass resolves from its preloaded snapshot instead of one query per file.
	GetByFilePathCalls atomic.Int64
	// Per-method error injection; zero value = no error (backward compatible)
	ErrCreate               error
	ErrUpdate               error
	ErrDelete               error
	GetByFileFingerprintErr error
	ListByLibraryIDErr      error
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
	r.items[string(item.ID)] = cloneMediaItem(item)
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
	r.items[string(item.ID)] = cloneMediaItem(item)
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
	r.GetByFilePathCalls.Add(1)
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.File != nil && item.File.Path == path {
			return cloneMediaItem(item), nil
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
			return cloneMediaItem(item), nil
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
	return cloneMediaItem(item), nil
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
				row.StreamProbeVersion = item.File.StreamProbeVersion
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
