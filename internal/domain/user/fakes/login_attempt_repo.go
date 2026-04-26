package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/user"
)

// FakeLoginAttemptRepository is an in-memory LoginAttemptRepository for tests.
type FakeLoginAttemptRepository struct {
	mu      sync.Mutex
	entries map[string]*user.LoginAttempt
	Err     error
}

// NewFakeLoginAttemptRepository returns an empty fake repository.
func NewFakeLoginAttemptRepository() *FakeLoginAttemptRepository {
	return &FakeLoginAttemptRepository{
		entries: make(map[string]*user.LoginAttempt),
	}
}

func (r *FakeLoginAttemptRepository) IsLocked(_ context.Context, username string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	e, ok := r.entries[username]
	if !ok || e.LockedUntil == nil {
		return false, nil
	}
	if time.Now().Before(*e.LockedUntil) {
		return true, nil
	}
	// Expired lock — clean up.
	delete(r.entries, username)
	return false, nil
}

func (r *FakeLoginAttemptRepository) Record(_ context.Context, username string, maxFails int, window time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	e, ok := r.entries[username]
	if !ok {
		e = &user.LoginAttempt{Username: username}
		r.entries[username] = e
	}
	now := time.Now()
	// Reset the counter when the previous window has expired.
	if !e.UpdatedAt.IsZero() && now.Sub(e.UpdatedAt) > window {
		e.AttemptCount = 0
		e.LockedUntil = nil
	}
	e.AttemptCount++
	e.UpdatedAt = now
	if e.AttemptCount >= maxFails {
		until := now.Add(window)
		e.LockedUntil = &until
	}
	return nil
}

func (r *FakeLoginAttemptRepository) Reset(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.entries, username)
	return nil
}

var _ user.LoginAttemptRepository = (*FakeLoginAttemptRepository)(nil)
