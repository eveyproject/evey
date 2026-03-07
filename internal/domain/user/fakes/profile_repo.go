package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type FakeProfileRepository struct {
	mu    sync.RWMutex
	items map[string]*user.Profile
}

func NewFakeProfileRepository() *FakeProfileRepository {
	return &FakeProfileRepository{items: make(map[string]*user.Profile)}
}

func (r *FakeProfileRepository) Create(_ context.Context, p *user.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[p.ID] = p
	return nil
}

func (r *FakeProfileRepository) GetByID(_ context.Context, id string) (*user.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakeProfileRepository) GetByUserID(_ context.Context, userID string) ([]*user.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*user.Profile
	for _, p := range r.items {
		if p.UserID == userID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *FakeProfileRepository) Update(_ context.Context, p *user.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[p.ID]; !ok {
		return ErrNotFound
	}
	r.items[p.ID] = p
	return nil
}

func (r *FakeProfileRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
