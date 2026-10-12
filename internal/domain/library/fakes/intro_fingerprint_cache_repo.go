package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// FakeIntroFingerprintCacheRepository is an in-memory replacement for the
// pHash fingerprint cache used in service tests. Set GetErr / UpsertErr to
// simulate adapter failures without losing the in-memory store.
type FakeIntroFingerprintCacheRepository struct {
	mu        sync.RWMutex
	items     map[string]*library.IntroFingerprintCacheEntry
	GetErr    error
	UpsertErr error
}

func NewFakeIntroFingerprintCacheRepository() *FakeIntroFingerprintCacheRepository {
	return &FakeIntroFingerprintCacheRepository{items: make(map[string]*library.IntroFingerprintCacheEntry)}
}

func (r *FakeIntroFingerprintCacheRepository) Get(_ context.Context, mediaItemID string) (*library.IntroFingerprintCacheEntry, error) {
	if r.GetErr != nil {
		return nil, r.GetErr
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.items[mediaItemID]
	if !ok {
		return nil, shared.ErrNotFound
	}
	cp := *e
	cp.PhashTiles = append([]uint64(nil), e.PhashTiles...)
	cp.PhashTimestamps = append([]int64(nil), e.PhashTimestamps...)
	return &cp, nil
}

func (r *FakeIntroFingerprintCacheRepository) Upsert(_ context.Context, entry *library.IntroFingerprintCacheEntry) error {
	if r.UpsertErr != nil {
		return r.UpsertErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *entry
	cp.PhashTiles = append([]uint64(nil), entry.PhashTiles...)
	cp.PhashTimestamps = append([]int64(nil), entry.PhashTimestamps...)
	if cp.ExtractedAt.IsZero() {
		cp.ExtractedAt = time.Now()
	}
	r.items[entry.MediaItemID] = &cp
	return nil
}

func (r *FakeIntroFingerprintCacheRepository) Delete(_ context.Context, mediaItemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, mediaItemID)
	return nil
}

func (r *FakeIntroFingerprintCacheRepository) DeleteByAlgoVersionBelow(_ context.Context, version int16) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted int64
	for id, e := range r.items {
		if e.AlgoVersion < version {
			delete(r.items, id)
			deleted++
		}
	}
	return deleted, nil
}

// Count returns the number of cached entries (useful in tests to assert
// cache hits/misses without going through the repo interface).
func (r *FakeIntroFingerprintCacheRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
