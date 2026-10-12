package fakes

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

var ErrNotFound = shared.ErrNotFound

type FakeUserRepository struct {
	mu       sync.RWMutex
	items    map[string]*user.User
	Err      error
	guardErr error
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
	r.items[string(u.ID)] = cloneUser(u)
	return nil
}

// GuardErr, when set, is returned by the guarded delete/disable - for tests
// exercising the service's error path without touching the generic Err.
func (r *FakeUserRepository) SetGuardErr(err error) {
	r.mu.Lock()
	r.guardErr = err
	r.mu.Unlock()
}

// DeleteIfNotLastAdmin mirrors the SQL guard: refuse when the target is an
// enabled admin and no other enabled admin remains, else delete. The fake's
// mutex makes the check-then-act atomic like the advisory lock does.
func (r *FakeUserRepository) DeleteIfNotLastAdmin(_ context.Context, id shared.UserID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.guardErr != nil {
		return r.guardErr
	}
	u, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	if u.IsAdmin && u.Enabled && r.countEnabledAdminsLocked() <= 1 {
		return user.ErrLastEnabledAdmin
	}
	delete(r.items, string(id))
	return nil
}

// DisableIfNotLastAdmin mirrors the SQL guard for the disable mutation.
func (r *FakeUserRepository) DisableIfNotLastAdmin(_ context.Context, id shared.UserID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.guardErr != nil {
		return r.guardErr
	}
	u, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	if u.IsAdmin && u.Enabled && r.countEnabledAdminsLocked() <= 1 {
		return user.ErrLastEnabledAdmin
	}
	u.Enabled = false
	return nil
}

func (r *FakeUserRepository) countEnabledAdminsLocked() int {
	n := 0
	for _, u := range r.items {
		if u.IsAdmin && u.Enabled {
			n++
		}
	}
	return n
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
	return cloneUser(u), nil
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
		// Same contract as the Postgres repo: lower(username) = lower($1).
		if strings.EqualFold(u.Username, username) {
			return cloneUser(u), nil
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
	r.items[string(u.ID)] = cloneUser(u)
	return nil
}

func (r *FakeUserRepository) TouchLastLogin(_ context.Context, id shared.UserID, at time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.items[string(id)]; ok {
		t := at
		u.LastLogin = &t
	}
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
			return cloneUser(u), nil
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
			return cloneUser(u), nil
		}
	}
	return nil, ErrNotFound
}

// LinkOIDCIdentity mirrors the SQL contract: only an account without an SSO
// identity is linked, and an identity already held elsewhere is a conflict.
func (r *FakeUserRepository) LinkOIDCIdentity(_ context.Context, id shared.UserID, issuer, subject string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	if u.OIDCSubject != nil {
		return shared.ErrConflict
	}
	for _, other := range r.items {
		if other.OIDCIssuer != nil && *other.OIDCIssuer == issuer && other.OIDCSubject != nil && *other.OIDCSubject == subject {
			return shared.ErrConflict
		}
	}
	iss, sub := issuer, subject
	u.OIDCIssuer, u.OIDCSubject = &iss, &sub
	return nil
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

func (r *FakeUserRepository) CountEnabledAdmins(_ context.Context) (int, error) {
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
		if u.IsAdmin && u.Enabled {
			count++
		}
	}
	return count, nil
}

// cloneUser detaches the stored record from the caller's pointer.
//
// A fake that hands back the SAME pointer it stores makes persistence
// untestable: a service that loads a user, mutates it in memory and never calls
// Update is indistinguishable from one that persists correctly, because the
// test reads back the very object the service mutated. Mutation testing proved
// this concretely - deleting the Update call from ChangePassword, DisableUser,
// EnableUser and SetUserBitrateLimit left the entire suite green.
//
// A real repository serialises, so the fake must too. Shallow copy: User holds
// no mutable reference fields today, and a shallow copy already breaks the
// aliasing that hid the bug.
func cloneUser(u *user.User) *user.User {
	if u == nil {
		return nil
	}
	cp := *u
	return &cp
}
