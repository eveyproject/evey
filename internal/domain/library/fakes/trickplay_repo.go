package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeTrickplayRepository struct {
	mu    sync.RWMutex
	items map[string]*library.TrickplayManifest
	Err   error
}

func NewFakeTrickplayRepository() *FakeTrickplayRepository {
	return &FakeTrickplayRepository{items: make(map[string]*library.TrickplayManifest)}
}

func (r *FakeTrickplayRepository) Create(_ context.Context, m *library.TrickplayManifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[m.MediaItemID] = m
	return nil
}

func (r *FakeTrickplayRepository) GetByMediaItemID(_ context.Context, mediaItemID string) (*library.TrickplayManifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	m, ok := r.items[mediaItemID]
	if !ok {
		return nil, library.ErrNotFound
	}
	return m, nil
}

func (r *FakeTrickplayRepository) Delete(_ context.Context, mediaItemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, mediaItemID)
	return nil
}

func (r *FakeTrickplayRepository) ListMissingMediaItemIDs(_ context.Context) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return nil, nil
}
