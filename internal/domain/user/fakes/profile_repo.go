package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type FakeProfileRepository struct {
	mu    sync.RWMutex
	items map[string]*user.Profile
	Err   error
}

func NewFakeProfileRepository() *FakeProfileRepository {
	return &FakeProfileRepository{items: make(map[string]*user.Profile)}
}

func (r *FakeProfileRepository) Create(_ context.Context, p *user.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[p.ID] = p
	return nil
}

func (r *FakeProfileRepository) GetByID(_ context.Context, id string) (*user.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakeProfileRepository) GetByUserID(_ context.Context, userID string) ([]*user.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
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
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[p.ID]; !ok {
		return ErrNotFound
	}
	r.items[p.ID] = p
	return nil
}

func (r *FakeProfileRepository) GetAll(_ context.Context) ([]*user.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	out := make([]*user.Profile, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, p)
	}
	return out, nil
}

func (r *FakeProfileRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
