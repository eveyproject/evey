package fakes

import (
	"context"
	"sync"
	"time"
)

type FakeLibraryAccessRepository struct {
	mu    sync.RWMutex
	items map[string]map[string]time.Time // userID -> libraryID -> grantedAt
	Err   error
}

func NewFakeLibraryAccessRepository() *FakeLibraryAccessRepository {
	return &FakeLibraryAccessRepository{items: make(map[string]map[string]time.Time)}
}

func (r *FakeLibraryAccessRepository) Grant(_ context.Context, userID, libraryID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.items[userID] == nil {
		r.items[userID] = make(map[string]time.Time)
	}
	r.items[userID][libraryID] = time.Now()
	return nil
}

func (r *FakeLibraryAccessRepository) Revoke(_ context.Context, userID, libraryID string) error {
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

func (r *FakeLibraryAccessRepository) GetByUserID(_ context.Context, userID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	libs := r.items[userID]
	if len(libs) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(libs))
	for libID := range libs {
		result = append(result, libID)
	}
	return result, nil
}

func (r *FakeLibraryAccessRepository) HasAccess(_ context.Context, userID, libraryID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return false, r.Err
	}
	libs := r.items[userID]
	if len(libs) == 0 {
		return true, nil // no restrictions = full access
	}
	_, ok := libs[libraryID]
	return ok, nil
}

func (r *FakeLibraryAccessRepository) ListUsersWithRestrictions(_ context.Context) (map[string][]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[string][]string, len(r.items))
	for userID, libs := range r.items {
		ids := make([]string, 0, len(libs))
		for libID := range libs {
			ids = append(ids, libID)
		}
		result[userID] = ids
	}
	return result, nil
}

func (r *FakeLibraryAccessRepository) DeleteByUserID(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, userID)
	return nil
}
