package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type FakePasswordResetTokenRepository struct {
	mu    sync.RWMutex
	items map[string]*user.PasswordResetToken
	Err   error
}

func NewFakePasswordResetTokenRepository() *FakePasswordResetTokenRepository {
	return &FakePasswordResetTokenRepository{items: make(map[string]*user.PasswordResetToken)}
}

func (r *FakePasswordResetTokenRepository) Create(_ context.Context, t *user.PasswordResetToken) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[t.ID] = t
	return nil
}

func (r *FakePasswordResetTokenRepository) GetByTokenHash(_ context.Context, hash string) (*user.PasswordResetToken, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.items {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (r *FakePasswordResetTokenRepository) MarkUsed(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.items[id]; ok {
		t.Used = true
	}
	return nil
}

func (r *FakePasswordResetTokenRepository) DeleteByUserID(_ context.Context, userID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, t := range r.items {
		if t.UserID == userID {
			delete(r.items, id)
		}
	}
	return nil
}

func (r *FakePasswordResetTokenRepository) DeleteExpired(_ context.Context) error {
	if r.Err != nil {
		return r.Err
	}
	return nil
}
