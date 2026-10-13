package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeTrickplayRepository struct {
	mu       sync.RWMutex
	items    map[string]*library.TrickplayManifest
	failures []TrickplayFailure
	Err      error
	// CurrentFile is the size and mtime the item's file has now, for
	// DeleteOfReplacedFile.
	CurrentFile func(shared.MediaItemID) (int64, time.Time, bool)
}

func NewFakeTrickplayRepository() *FakeTrickplayRepository {
	return &FakeTrickplayRepository{items: make(map[string]*library.TrickplayManifest)}
}

func (r *FakeTrickplayRepository) Create(_ context.Context, m *library.TrickplayManifest) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(m.MediaItemID)] = cloneTrickplayManifest(m)
	return nil
}

func (r *FakeTrickplayRepository) GetByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) (*library.TrickplayManifest, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	m, ok := r.items[string(mediaItemID)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneTrickplayManifest(m), nil
}

// DeleteOfReplacedFile drops the manifest unless CurrentFile names the very
// file it was made from; without CurrentFile every stamped manifest is kept.
func (r *FakeTrickplayRepository) DeleteOfReplacedFile(_ context.Context, mediaItemID shared.MediaItemID) (bool, error) {
	if r.Err != nil {
		return false, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.items[string(mediaItemID)]
	if !ok {
		return false, nil
	}
	if r.CurrentFile != nil && m.FileSize > 0 {
		if size, mod, ok := r.CurrentFile(mediaItemID); ok && size == m.FileSize && mod.Equal(m.ModTime) {
			return false, nil
		}
	} else if m.FileSize > 0 {
		return false, nil
	}
	delete(r.items, string(mediaItemID))
	return true, nil
}

func (r *FakeTrickplayRepository) GetByMediaItemIDs(_ context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID]*library.TrickplayManifest, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[shared.MediaItemID]*library.TrickplayManifest, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if m, ok := r.items[string(id)]; ok {
			result[id] = cloneTrickplayManifest(m)
		}
	}
	return result, nil
}

func (r *FakeTrickplayRepository) Delete(_ context.Context, mediaItemID shared.MediaItemID) error {
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

func (r *FakeTrickplayRepository) ListMissingMediaItemIDs(_ context.Context, _ int) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return nil, nil
}

// TrickplayFailure is one RecordFailure call, exposed for test assertions.
type TrickplayFailure struct {
	MediaItemID shared.MediaItemID
	FileSize    int64
	ModTime     time.Time
	Reason      string
}

func (r *FakeTrickplayRepository) RecordFailure(_ context.Context, mediaItemID shared.MediaItemID, fileSize int64, modTime time.Time, reason string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = append(r.failures, TrickplayFailure{
		MediaItemID: mediaItemID, FileSize: fileSize, ModTime: modTime, Reason: reason,
	})
	return nil
}

// Failures snapshots the recorded generation failures.
func (r *FakeTrickplayRepository) Failures() []TrickplayFailure {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]TrickplayFailure(nil), r.failures...)
}

// cloneTrickplayManifest detaches the stored record from the caller's
// pointer, so a service that mutates a manifest in memory without persisting
// it again is distinguishable from one that does. See cloneUser (user fakes)
// for the full rationale. TrickplayManifest holds only scalar fields, so a
// shallow copy is enough.
func cloneTrickplayManifest(m *library.TrickplayManifest) *library.TrickplayManifest {
	if m == nil {
		return nil
	}
	cp := *m
	return &cp
}
