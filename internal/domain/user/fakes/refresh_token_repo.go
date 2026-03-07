package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type FakeRefreshTokenRepository struct {
	mu    sync.RWMutex
	items map[string]*user.RefreshToken
}

func NewFakeRefreshTokenRepository() *FakeRefreshTokenRepository {
	return &FakeRefreshTokenRepository{items: make(map[string]*user.RefreshToken)}
}

func (r *FakeRefreshTokenRepository) Create(_ context.Context, t *user.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[t.ID] = t
	return nil
}

func (r *FakeRefreshTokenRepository) GetByTokenHash(_ context.Context, hash string) (*user.RefreshToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.items {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeRefreshTokenRepository) DeleteByID(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
	return nil
}

func (r *FakeRefreshTokenRepository) DeleteByUserID(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, t := range r.items {
		if t.UserID == userID {
			delete(r.items, id)
		}
	}
	return nil
}

func (r *FakeRefreshTokenRepository) DeleteExpired(_ context.Context) error {
	return nil
}
