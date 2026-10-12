package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakePlaylistRepository struct {
	mu    sync.RWMutex
	items map[string]*playlist.Playlist
	Err   error
}

func NewFakePlaylistRepository() *FakePlaylistRepository {
	return &FakePlaylistRepository{items: make(map[string]*playlist.Playlist)}
}

func (r *FakePlaylistRepository) Create(_ context.Context, pl *playlist.Playlist) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[pl.ID] = pl
	return nil
}

func (r *FakePlaylistRepository) GetByID(_ context.Context, id string) (*playlist.Playlist, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	pl, ok := r.items[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return pl, nil
}

func (r *FakePlaylistRepository) List(_ context.Context, profileID string) ([]*playlist.Playlist, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playlist.Playlist
	for _, pl := range r.items {
		if string(pl.ProfileID) == profileID {
			result = append(result, pl)
		}
	}
	return result, nil
}

func (r *FakePlaylistRepository) Update(_ context.Context, pl *playlist.Playlist) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[pl.ID]; !ok {
		return shared.ErrNotFound
	}
	r.items[pl.ID] = pl
	return nil
}

func (r *FakePlaylistRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return shared.ErrNotFound
	}
	delete(r.items, id)
	return nil
}
