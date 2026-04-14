package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
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
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[m.MediaItemID] = m
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
	return m, nil
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

func (r *FakeTrickplayRepository) ListMissingMediaItemIDs(_ context.Context) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return nil, nil
}
