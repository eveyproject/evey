package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

type FakeAPIKeyRepository struct {
	mu    sync.RWMutex
	items map[string]*user.APIKeyModel
	Err   error
}

func NewFakeAPIKeyRepository() *FakeAPIKeyRepository {
	return &FakeAPIKeyRepository{items: make(map[string]*user.APIKeyModel)}
}

func (r *FakeAPIKeyRepository) Create(_ context.Context, key *user.APIKeyModel) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[key.ID] = key
	return nil
}

func (r *FakeAPIKeyRepository) GetByKeyHash(_ context.Context, hash string) (*user.APIKeyModel, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	for _, k := range r.items {
		if k.KeyHash == hash {
			// Return a COPY, like the real Postgres repo hydrates a fresh row.
			// Handing back the stored pointer lets a caller read fields (e.g.
			// LastUsedAt in the auth interceptor's throttle) while the detached
			// UpdateLastUsed goroutine writes the same object - a data race.
			cp := *k
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeAPIKeyRepository) ListByUserID(_ context.Context, userID shared.UserID) ([]*user.APIKeyModel, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*user.APIKeyModel
	for _, k := range r.items {
		if k.UserID == userID {
			result = append(result, k)
		}
	}
	return result, nil
}

func (r *FakeAPIKeyRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	// Mirrors the Postgres repository: a delete that hits no row is NotFound.
	if _, ok := r.items[id]; !ok {
		return shared.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *FakeAPIKeyRepository) UpdateLastUsed(_ context.Context, id string, t time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if k, ok := r.items[id]; ok {
		k.LastUsedAt = &t
	}
	return nil
}
