package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
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
	return nil, shared.ErrNotFound
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

func (r *FakePasswordResetTokenRepository) DeleteByUserID(_ context.Context, userID shared.UserID) error {
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
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, t := range r.items {
		if t.ExpiresAt.Before(now) || t.Used {
			delete(r.items, id)
		}
	}
	return nil
}

func (r *FakePasswordResetTokenRepository) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

func (r *FakePasswordResetTokenRepository) Get(id string) *user.PasswordResetToken {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[id]
}
