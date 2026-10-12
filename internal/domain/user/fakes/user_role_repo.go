package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

type FakeUserRoleRepository struct {
	mu       sync.RWMutex
	assigned map[string][]string
	roles    *FakeRoleRepository
	Err      error
}

func NewFakeUserRoleRepository(roles *FakeRoleRepository) *FakeUserRoleRepository {
	return &FakeUserRoleRepository{
		assigned: make(map[string][]string),
		roles:    roles,
	}
}

func (r *FakeUserRoleRepository) AssignRole(_ context.Context, userID shared.UserID, roleID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.assigned[string(userID)] = append(r.assigned[string(userID)], roleID)
	return nil
}

func (r *FakeUserRoleRepository) RemoveRole(_ context.Context, userID shared.UserID, roleID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	ids := r.assigned[string(userID)]
	for i, id := range ids {
		if id == roleID {
			r.assigned[string(userID)] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}

func (r *FakeUserRoleRepository) GetRolesByUserID(ctx context.Context, userID shared.UserID) ([]*user.Role, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*user.Role
	for _, roleID := range r.assigned[string(userID)] {
		role, err := r.roles.GetByID(ctx, roleID)
		if err == nil {
			result = append(result, role)
		}
	}
	return result, nil
}
