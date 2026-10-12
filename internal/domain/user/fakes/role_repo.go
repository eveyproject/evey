package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/user"
)

type FakeRoleRepository struct {
	mu    sync.RWMutex
	items map[string]*user.Role
	Err   error
}

func NewFakeRoleRepository() *FakeRoleRepository {
	return &FakeRoleRepository{items: make(map[string]*user.Role)}
}

func (r *FakeRoleRepository) Create(_ context.Context, role *user.Role) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[role.ID] = role
	return nil
}

func (r *FakeRoleRepository) GetByID(_ context.Context, id string) (*user.Role, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	role, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return role, nil
}

func (r *FakeRoleRepository) GetByName(_ context.Context, name string) (*user.Role, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	for _, role := range r.items {
		if role.Name == name {
			return role, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeRoleRepository) List(_ context.Context) ([]*user.Role, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make([]*user.Role, 0, len(r.items))
	for _, role := range r.items {
		result = append(result, role)
	}
	return result, nil
}
