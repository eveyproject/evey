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
}

func NewFakeSearchIndex() *FakeSearchIndex {
	return &FakeSearchIndex{items: make(map[string]*library.MediaItem)}
}

func (s *FakeSearchIndex) IndexMediaItem(_ context.Context, item *library.MediaItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[item.ID] = item
	return nil
}

func (s *FakeSearchIndex) RemoveMediaItem(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	return nil
}

func (s *FakeSearchIndex) Search(_ context.Context, query string, _ map[string]string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	q := strings.ToLower(query)
	for _, item := range s.items {
		if strings.Contains(strings.ToLower(item.Title), q) {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}
