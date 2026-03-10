package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/playlist"
)

type FakePlaylistRepository struct {
	mu    sync.RWMutex
	items map[string]*playlist.Playlist
}

func NewFakePlaylistRepository() *FakePlaylistRepository {
	return &FakePlaylistRepository{items: make(map[string]*playlist.Playlist)}
}

func (r *FakePlaylistRepository) Create(_ context.Context, pl *playlist.Playlist) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[pl.ID] = pl
	return nil
}

func (r *FakePlaylistRepository) GetByID(_ context.Context, id string) (*playlist.Playlist, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pl, ok := r.items[id]
	if !ok {
		return nil, playlist.ErrNotFound
	}
	return pl, nil
}

func (r *FakePlaylistRepository) List(_ context.Context, userID string) ([]*playlist.Playlist, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*playlist.Playlist
	for _, pl := range r.items {
		if pl.UserID == userID {
			result = append(result, pl)
		}
	}
	return result, nil
}

func (r *FakePlaylistRepository) Update(_ context.Context, pl *playlist.Playlist) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[pl.ID]; !ok {
		return playlist.ErrNotFound
	}
	r.items[pl.ID] = pl
	return nil
}

func (r *FakePlaylistRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return playlist.ErrNotFound
	}
	delete(r.items, id)
	return nil
}
