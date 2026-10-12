package fakes

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakeSearchIndex struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	Err   error
	// LastSearchLimit records the limit of the most recent Search call so tests
	// can assert the handler clamped it. Atomic so it never races the RLock.
	LastSearchLimit atomic.Int64
	// ClearCalls counts Clear invocations so the reconcile test can assert an
	// oversized index was wiped before the rebuild.
	ClearCalls atomic.Int64
}

func NewFakeSearchIndex() *FakeSearchIndex {
	return &FakeSearchIndex{items: make(map[string]*library.MediaItem)}
}

func (s *FakeSearchIndex) IndexMediaItem(_ context.Context, item *library.MediaItem) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	s.items[string(item.ID)] = cloneMediaItem(item)
	return nil
}

func (s *FakeSearchIndex) IndexMediaItemsBatch(_ context.Context, items []*library.MediaItem) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.items[string(item.ID)] = cloneMediaItem(item)
	}
	return nil
}

// Indexed returns how many distinct media items are currently in the index -
// a test accessor for asserting reindex/index behaviour.
func (s *FakeSearchIndex) Indexed() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Get returns the indexed item by id - a test accessor for asserting what was
// pushed to the index (e.g. attached translations). Returns a clone so a test
// mutating the result can't silently corrupt the index it's meant to observe.
func (s *FakeSearchIndex) Get(id string) (*library.MediaItem, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.items[id]
	if !ok {
		return nil, false
	}
	return cloneMediaItem(it), true
}

func (s *FakeSearchIndex) RemoveMediaItem(_ context.Context, id string) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	delete(s.items, id)
	return nil
}

func (s *FakeSearchIndex) Clear(_ context.Context) error {
	s.ClearCalls.Add(1)
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]*library.MediaItem)
	return nil
}

func (s *FakeSearchIndex) Search(_ context.Context, query string, _ map[string]string, _ []string, limit int64) ([]library.SearchHit, int64, error) {
	s.LastSearchLimit.Store(limit)
	if s.Err != nil {
		return nil, 0, s.Err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Err != nil {
		return nil, 0, s.Err
	}
	var hits []library.SearchHit
	q := strings.ToLower(query)
	for _, item := range s.items {
		if strings.Contains(strings.ToLower(item.Title), q) {
			hit := library.SearchHit{
				ID:        string(item.ID),
				LibraryID: item.LibraryID,
				Title:     item.Title,
				Type:      string(item.Type),
			}
			if item.Catalog != nil {
				hit.Year = item.Catalog.Year
				hit.Genres = item.Catalog.Genres
				hit.Tags = item.Catalog.Tags
			}
			if item.Classification != nil {
				hit.OfficialRating = item.Classification.OfficialRating
			}
			hits = append(hits, hit)
		}
	}
	if limit > 0 && int64(len(hits)) > limit {
		hits = hits[:limit]
	}
	return hits, int64(len(hits)), nil
}
