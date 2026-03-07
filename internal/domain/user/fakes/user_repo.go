package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
)

var ErrNotFound = errors.New("not found")

type FakeUserRepository struct {
	mu    sync.RWMutex
	items map[string]*user.User
}

func NewFakeUserRepository() *FakeUserRepository {
	return &FakeUserRepository{items: make(map[string]*user.User)}
}

func (r *FakeUserRepository) Create(_ context.Context, u *user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[u.ID] = u
	return nil
}

func (r *FakeUserRepository) GetByID(_ context.Context, id string) (*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (r *FakeUserRepository) GetByUsername(_ context.Context, username string) (*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.items {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeUserRepository) GetByOIDCSubject(_ context.Context, issuer, subject string) (*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.items {
		if u.OIDCSubject != nil && *u.OIDCSubject == subject && u.OIDCIssuer != nil && *u.OIDCIssuer == issuer {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeUserRepository) List(_ context.Context) ([]*user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*user.User, 0, len(r.items))
	for _, u := range r.items {
		result = append(result, u)
	}
	return result, nil
}

func (r *FakeUserRepository) Update(_ context.Context, u *user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[u.ID]; !ok {
		return ErrNotFound
	}
	r.items[u.ID] = u
	return nil
}

func (r *FakeUserRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *FakeUserRepository) CountAdmins(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, u := range r.items {
		if u.IsAdmin {
			count++
		}
	}
	return count, nil
}
