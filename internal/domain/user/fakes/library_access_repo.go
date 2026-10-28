package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeLibraryAccessRepository struct {
	mu         sync.RWMutex
	items      map[shared.UserID]map[shared.LibraryID]time.Time
	restricted map[shared.UserID]bool
	Err        error
}

func NewFakeLibraryAccessRepository() *FakeLibraryAccessRepository {
	return &FakeLibraryAccessRepository{
		items:      make(map[shared.UserID]map[shared.LibraryID]time.Time),
		restricted: make(map[shared.UserID]bool),
	}
}

func (r *FakeLibraryAccessRepository) Grant(_ context.Context, userID shared.UserID, libraryID shared.LibraryID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.items[userID] == nil {
		r.items[userID] = make(map[shared.LibraryID]time.Time)
	}
	r.items[userID][libraryID] = time.Now()
	r.restricted[userID] = true
	return nil
}

func (r *FakeLibraryAccessRepository) GrantMany(_ context.Context, userID shared.UserID, libraryIDs []shared.LibraryID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.items[userID] == nil {
		r.items[userID] = make(map[shared.LibraryID]time.Time)
	}
	now := time.Now()
	for _, id := range libraryIDs {
		r.items[userID][id] = now
	}
	r.restricted[userID] = true
	return nil
}

// ReplaceAccess atomically swaps userID's whole grant set for libraryIDs.
// Err is checked before any mutation, so a simulated repo failure leaves the
// existing grants completely untouched - the fail-closed contract the real
// transactional adapter provides via a single tx.
func (r *FakeLibraryAccessRepository) ReplaceAccess(_ context.Context, userID shared.UserID, libraryIDs []shared.LibraryID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	now := time.Now()
	next := make(map[shared.LibraryID]time.Time, len(libraryIDs))
	for _, id := range libraryIDs {
		next[id] = now
	}
	r.items[userID] = next
	r.restricted[userID] = true
	return nil
}

func (r *FakeLibraryAccessRepository) Revoke(_ context.Context, userID shared.UserID, libraryID shared.LibraryID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if libs, ok := r.items[userID]; ok {
		delete(libs, libraryID)
		if len(libs) == 0 {
			delete(r.items, userID)
		}
	}
	return nil
}

func (r *FakeLibraryAccessRepository) GetByUserID(_ context.Context, userID shared.UserID) ([]shared.LibraryID, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	libs := r.items[userID]
	if len(libs) == 0 {
		return nil, nil
	}
	result := make([]shared.LibraryID, 0, len(libs))
	for libID := range libs {
		result = append(result, libID)
	}
	return result, nil
}

func (r *FakeLibraryAccessRepository) HasAccess(_ context.Context, userID shared.UserID, libraryID shared.LibraryID) (bool, error) {
	if r.Err != nil {
		return false, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return false, r.Err
	}
	if !r.restricted[userID] {
		return true, nil
	}
	_, ok := r.items[userID][libraryID]
	return ok, nil
}

func (r *FakeLibraryAccessRepository) IsRestricted(_ context.Context, userID shared.UserID) (bool, error) {
	if r.Err != nil {
		return false, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.restricted[userID], nil
}

func (r *FakeLibraryAccessRepository) ListUsersWithRestrictions(_ context.Context) (map[shared.UserID][]shared.LibraryID, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[shared.UserID][]shared.LibraryID, len(r.items))
	for userID, libs := range r.items {
		ids := make([]shared.LibraryID, 0, len(libs))
		for libID := range libs {
			ids = append(ids, libID)
		}
		result[userID] = ids
	}
	return result, nil
}

func (r *FakeLibraryAccessRepository) DeleteByUserID(_ context.Context, userID shared.UserID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, userID)
	delete(r.restricted, userID)
	return nil
}
