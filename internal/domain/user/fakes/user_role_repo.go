package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type FakeUserRoleRepository struct {
	mu       sync.RWMutex
	assigned map[string][]string
	roles    *FakeRoleRepository
}

func NewFakeUserRoleRepository(roles *FakeRoleRepository) *FakeUserRoleRepository {
	return &FakeUserRoleRepository{
		assigned: make(map[string][]string),
		roles:    roles,
	}
}

func (r *FakeUserRoleRepository) AssignRole(_ context.Context, userID, roleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assigned[userID] = append(r.assigned[userID], roleID)
	return nil
}

func (r *FakeUserRoleRepository) RemoveRole(_ context.Context, userID, roleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.assigned[userID]
	for i, id := range ids {
		if id == roleID {
			r.assigned[userID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}

func (r *FakeUserRoleRepository) GetRolesByUserID(ctx context.Context, userID string) ([]*user.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*user.Role
	for _, roleID := range r.assigned[userID] {
		role, err := r.roles.GetByID(ctx, roleID)
		if err == nil {
			result = append(result, role)
		}
	}
	return result, nil
}
