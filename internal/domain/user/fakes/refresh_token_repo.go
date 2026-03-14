package fakes

import (
	"context"
	"sync"

	"github.com/lib/pq"

	"github.com/gravity-zero/evey/internal/domain/user"
)

// ErrDuplicateTokenHash is a *pq.Error with code 23505 (unique_violation), matching real postgres behaviour.
var ErrDuplicateTokenHash = &pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"}

type FakeRefreshTokenRepository struct {
	mu    sync.RWMutex
	items map[string]*user.RefreshToken
	Err   error
}

func NewFakeRefreshTokenRepository() *FakeRefreshTokenRepository {
	return &FakeRefreshTokenRepository{items: make(map[string]*user.RefreshToken)}
}

func (r *FakeRefreshTokenRepository) Create(_ context.Context, t *user.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	for _, existing := range r.items {
		if existing.TokenHash == t.TokenHash {
			return ErrDuplicateTokenHash
		}
	}
	r.items[t.ID] = t
	return nil
}

func (r *FakeRefreshTokenRepository) GetByTokenHash(_ context.Context, hash string) (*user.RefreshToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	for _, t := range r.items {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeRefreshTokenRepository) MarkUsed(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if t, ok := r.items[id]; ok {
		t.Used = true
	}
	return nil
}

func (r *FakeRefreshTokenRepository) DeleteByID(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, id)
	return nil
}

func (r *FakeRefreshTokenRepository) DeleteByUserID(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	for id, t := range r.items {
		if t.UserID == userID {
			delete(r.items, id)
		}
	}
	return nil
}

func (r *FakeRefreshTokenRepository) DeleteExpired(_ context.Context) error {
	if r.Err != nil {
		return r.Err
	}
	return nil
}
