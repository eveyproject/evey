package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type FakeInviteLinkRepository struct {
	mu    sync.RWMutex
	items map[string]*user.InviteLink
}

func NewFakeInviteLinkRepository() *FakeInviteLinkRepository {
	return &FakeInviteLinkRepository{items: make(map[string]*user.InviteLink)}
}

func (r *FakeInviteLinkRepository) Create(_ context.Context, link *user.InviteLink) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[link.ID] = link
	return nil
}

func (r *FakeInviteLinkRepository) GetByToken(_ context.Context, token string) (*user.InviteLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, l := range r.items {
		if l.Token == token {
			return l, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeInviteLinkRepository) ListByCreator(_ context.Context, creatorID string) ([]*user.InviteLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*user.InviteLink
	for _, l := range r.items {
		if l.CreatedBy == creatorID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (r *FakeInviteLinkRepository) IncrementUsedCount(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.items[id]; ok {
		l.UsedCount++
	}
	return nil
}

func (r *FakeInviteLinkRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
	return nil
}
