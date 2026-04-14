package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

var ErrNotFound = shared.ErrNotFound

type FakeUserRepository struct {
	mu    sync.RWMutex
	items map[string]*user.User
	Err   error
}

func NewFakeUserRepository() *FakeUserRepository {
	return &FakeUserRepository{items: make(map[string]*user.User)}
}

func (r *FakeUserRepository) Create(_ context.Context, u *user.User) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(u.ID)] = u
	return nil
}

func (r *FakeUserRepository) GetByID(_ context.Context, id shared.UserID) (*user.User, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	u, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (r *FakeUserRepository) GetByUsername(_ context.Context, username string) (*user.User, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	for _, u := range r.items {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeUserRepository) List(_ context.Context) ([]*user.User, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make([]*user.User, 0, len(r.items))
	for _, u := range r.items {
		result = append(result, u)
	}
	return result, nil
}

func (r *FakeUserRepository) Update(_ context.Context, u *user.User) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[string(u.ID)]; !ok {
		return ErrNotFound
	}
	r.items[string(u.ID)] = u
	return nil
}

func (r *FakeUserRepository) Delete(_ context.Context, id shared.UserID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[string(id)]; !ok {
		return ErrNotFound
	}
	delete(r.items, string(id))
	return nil
}

func (r *FakeUserRepository) GetByEmail(_ context.Context, email string) (*user.User, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.items {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeUserRepository) GetByOIDCSubject(_ context.Context, issuer, subject string) (*user.User, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.items {
		if u.OIDCIssuer != nil && *u.OIDCIssuer == issuer && u.OIDCSubject != nil && *u.OIDCSubject == subject {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeUserRepository) CountAdmins(_ context.Context) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return 0, r.Err
	}
	count := 0
	for _, u := range r.items {
		if u.IsAdmin {
			count++
		}
	}
	return count, nil
}
