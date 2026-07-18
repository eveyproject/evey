package fakes

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

var ErrDuplicateTokenHash = fmt.Errorf("%w: duplicate key value violates unique constraint", shared.ErrConflict)

type FakeRefreshTokenRepository struct {
	mu    sync.RWMutex
	items map[string]*user.RefreshToken
	Err   error
}

func NewFakeRefreshTokenRepository() *FakeRefreshTokenRepository {
	return &FakeRefreshTokenRepository{items: make(map[string]*user.RefreshToken)}
}

func (r *FakeRefreshTokenRepository) Create(_ context.Context, t *user.RefreshToken) error {
	if r.Err != nil {
		return r.Err
	}
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
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	for _, t := range r.items {
		if t.TokenHash == hash {
			// Return a defensive copy so concurrent callers don't observe
			// in-flight Used=true writes from MarkUsedIfNotUsed (the real
			// Postgres adapter materializes a fresh struct per query, so
			// production has no such race; the fake only exposed it).
			cloned := *t
			return &cloned, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeRefreshTokenRepository) MarkUsedIfNotUsed(_ context.Context, id string) (bool, error) {
	if r.Err != nil {
		return false, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	t, ok := r.items[id]
	if !ok || t.Used {
		return false, nil
	}
	t.Used = true
	now := time.Now()
	t.UsedAt = &now
	return true, nil
}

func (r *FakeRefreshTokenRepository) DeleteByID(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, id)
	return nil
}

func (r *FakeRefreshTokenRepository) DeleteByUserID(_ context.Context, userID shared.UserID) error {
	if r.Err != nil {
		return r.Err
	}
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
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, t := range r.items {
		// Mirrors the SQL sweep: expired rows, plus rotated rows once the
		// grace window has long passed (they are only retained so a multi-tab
		// race loser can be recognised - see the app-layer grace).
		if t.ExpiresAt.Before(now) || (t.Used && t.UsedAt != nil && now.Sub(*t.UsedAt) > 10*time.Minute) {
			delete(r.items, id)
		}
	}
	return nil
}

func (r *FakeRefreshTokenRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
