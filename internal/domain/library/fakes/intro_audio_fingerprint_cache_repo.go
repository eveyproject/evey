package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// FakeIntroAudioFingerprintCacheRepository is an in-memory replacement for the
// Chromaprint fingerprint cache used in service tests. Set GetErr / UpsertErr to
// simulate adapter failures without losing the in-memory store.
type FakeIntroAudioFingerprintCacheRepository struct {
	mu        sync.RWMutex
	items     map[string]*library.IntroAudioFingerprintCacheEntry
	GetErr    error
	UpsertErr error
}

func NewFakeIntroAudioFingerprintCacheRepository() *FakeIntroAudioFingerprintCacheRepository {
	return &FakeIntroAudioFingerprintCacheRepository{items: make(map[string]*library.IntroAudioFingerprintCacheEntry)}
}

func (r *FakeIntroAudioFingerprintCacheRepository) Get(_ context.Context, mediaItemID string) (*library.IntroAudioFingerprintCacheEntry, error) {
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
	cp.Fingerprint = append([]uint32(nil), e.Fingerprint...)
	return &cp, nil
}

func (r *FakeIntroAudioFingerprintCacheRepository) Upsert(_ context.Context, entry *library.IntroAudioFingerprintCacheEntry) error {
	if r.UpsertErr != nil {
		return r.UpsertErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *entry
	cp.Fingerprint = append([]uint32(nil), entry.Fingerprint...)
	if cp.ExtractedAt.IsZero() {
		cp.ExtractedAt = time.Now()
	}
	r.items[entry.MediaItemID] = &cp
	return nil
}

func (r *FakeIntroAudioFingerprintCacheRepository) Delete(_ context.Context, mediaItemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, mediaItemID)
	return nil
}

func (r *FakeIntroAudioFingerprintCacheRepository) DeleteByAlgoVersionBelow(_ context.Context, version int16) (int64, error) {
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
func (r *FakeIntroAudioFingerprintCacheRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
