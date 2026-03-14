package fakes

import (
	"context"
	"strings"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	s.items[item.ID] = item
	return nil
}

func (s *FakeSearchIndex) RemoveMediaItem(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	delete(s.items, id)
	return nil
}

func (s *FakeSearchIndex) Search(_ context.Context, query string, _ map[string]string, limit int64) ([]library.SearchHit, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Err != nil {
		return nil, 0, s.Err
	}
	var hits []library.SearchHit
	q := strings.ToLower(query)
	for _, item := range s.items {
		if strings.Contains(strings.ToLower(item.Title), q) {
			hits = append(hits, library.SearchHit{
				ID:             item.ID,
				Title:          item.Title,
				Year:           item.Year,
				Type:           item.Type,
				Genres:         item.Genres,
				Tags:           item.Tags,
				OfficialRating: item.OfficialRating,
			})
		}
	}
	if limit > 0 && int64(len(hits)) > limit {
		hits = hits[:limit]
	}
	return hits, int64(len(hits)), nil
}
