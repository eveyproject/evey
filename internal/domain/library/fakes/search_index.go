package fakes

import (
	"context"
	"strings"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
)

type FakeSearchIndex struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	Err   error
}

func NewFakeSearchIndex() *FakeSearchIndex {
	return &FakeSearchIndex{items: make(map[string]*library.MediaItem)}
}

func (s *FakeSearchIndex) IndexMediaItem(_ context.Context, item *library.MediaItem, _ []string) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	s.items[string(item.ID)] = item
	return nil
}

func (s *FakeSearchIndex) IndexMediaItemsBatch(_ context.Context, items []*library.MediaItem) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.items[string(item.ID)] = item
	}
	return nil
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

func (s *FakeSearchIndex) Search(_ context.Context, query string, _ map[string]string, _ []string, limit int64) ([]library.SearchHit, int64, error) {
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
				Year:      item.Year,
				Type:      string(item.Type),
				Genres:    item.Genres,
				Tags:      item.Tags,
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
