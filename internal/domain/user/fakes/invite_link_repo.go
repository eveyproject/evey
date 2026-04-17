package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

type FakeInviteLinkRepository struct {
	mu    sync.RWMutex
	items map[string]*user.InviteLink
	Err   error
}

func NewFakeInviteLinkRepository() *FakeInviteLinkRepository {
	return &FakeInviteLinkRepository{items: make(map[string]*user.InviteLink)}
}

func (r *FakeInviteLinkRepository) Create(_ context.Context, link *user.InviteLink) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[link.ID] = link
	return nil
}

func (r *FakeInviteLinkRepository) GetByToken(_ context.Context, token string) (*user.InviteLink, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	for _, l := range r.items {
		if l.Token == token {
			return l, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeInviteLinkRepository) ListByCreator(_ context.Context, creatorID string) ([]*user.InviteLink, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*user.InviteLink
	for _, l := range r.items {
		if string(l.CreatedBy) == creatorID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (r *FakeInviteLinkRepository) IncrementUsedCount(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	l, ok := r.items[id]
	if !ok {
		return shared.ErrNotFound
	}
	if l.MaxUses > 0 && l.UsedCount >= l.MaxUses {
		return shared.ErrNotFound
	}
	l.UsedCount++
	return nil
}

func (r *FakeInviteLinkRepository) Delete(_ context.Context, id string) error {
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
