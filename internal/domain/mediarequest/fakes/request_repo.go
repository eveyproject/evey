// Package fakes holds in-memory doubles for the mediarequest ports. It brings
// this domain in line with every other one, which already ships its fakes here
// instead of re-declaring them inline in each test package.
package fakes

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/eveyproject/evey/internal/domain/mediarequest"
)

// FakeRepository is an in-memory mediarequest.Repository.
type FakeRepository struct {
	mu sync.Mutex

	byID map[string]*mediarequest.Request
	// order preserves insertion so the list methods can return newest-first
	// deterministically, which timestamps alone would not on a fast test.
	order []string

	// Created and Updated record the calls themselves, not just the resulting
	// state: a test asserting "nothing was persisted" needs to distinguish
	// "Create was never called" from "Create was called and overwrote".
	Created []*mediarequest.Request
	Updated []*mediarequest.Request

	// HasOpen forces HasOpenRequest's answer; Err, when set, is returned by
	// HasOpenRequest so the duplicate-check failure path stays reachable.
	HasOpen bool
	Err     error
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{byID: map[string]*mediarequest.Request{}}
}

func (f *FakeRepository) Create(_ context.Context, r *mediarequest.Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.byID[r.ID]; !exists {
		f.order = append(f.order, r.ID)
	}
	f.byID[r.ID] = r
	f.Created = append(f.Created, r)
	return nil
}

func (f *FakeRepository) GetByID(_ context.Context, id string) (*mediarequest.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.byID[id]; ok {
		return r, nil
	}
	return nil, mediarequest.ErrNotFound
}

func (f *FakeRepository) Update(_ context.Context, r *mediarequest.Request) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[r.ID]; !ok {
		return mediarequest.ErrNotFound
	}
	f.byID[r.ID] = r
	f.Updated = append(f.Updated, r)
	return nil
}

func (f *FakeRepository) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byID, id)
	for i, existing := range f.order {
		if existing == id {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
	return nil
}

func (f *FakeRepository) ListByProfile(_ context.Context, profileID string, limit, offset int) ([]*mediarequest.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*mediarequest.Request
	for _, r := range f.newestFirstLocked() {
		if string(r.ProfileID) == profileID {
			out = append(out, r)
		}
	}
	return page(out, limit, offset), nil
}

func (f *FakeRepository) ListAll(_ context.Context, statuses []mediarequest.Status, limit, offset int) ([]*mediarequest.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	wanted := make(map[mediarequest.Status]bool, len(statuses))
	for _, s := range statuses {
		wanted[s] = true
	}
	var out []*mediarequest.Request
	for _, r := range f.newestFirstLocked() {
		if len(wanted) == 0 || wanted[r.Status] {
			out = append(out, r)
		}
	}
	return page(out, limit, offset), nil
}

func (f *FakeRepository) HasOpenRequest(_ context.Context, _ string, _ mediarequest.MediaType, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.HasOpen, f.Err
}

// All returns every stored request, newest first. Test-only accessor.
func (f *FakeRepository) All() []*mediarequest.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.newestFirstLocked()
}

// newestFirstLocked walks the insertion order backwards. Sorting on timestamps
// would be flaky here - a test creates several requests within the same clock
// tick - so insertion order is the deterministic stand-in for "newest".
func (f *FakeRepository) newestFirstLocked() []*mediarequest.Request {
	out := make([]*mediarequest.Request, 0, len(f.order))
	for _, id := range slices.Backward(f.order) {
		if r, ok := f.byID[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

func page[T any](in []T, limit, offset int) []T {
	if offset >= len(in) {
		return nil
	}
	in = in[offset:]
	if limit > 0 && limit < len(in) {
		in = in[:limit]
	}
	return in
}

// FakeAccess is an in-memory mediarequest.Access.
type FakeAccess struct {
	mu sync.Mutex

	Global    bool
	Overrides map[string]bool
}

func NewFakeAccess(globalDefault bool) *FakeAccess {
	return &FakeAccess{Global: globalDefault, Overrides: map[string]bool{}}
}

func (f *FakeAccess) GlobalDefault(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Global, nil
}

func (f *FakeAccess) SetGlobalDefault(_ context.Context, v bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Global = v
	return nil
}

func (f *FakeAccess) Override(_ context.Context, userID string) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.Overrides[userID]
	return v, ok, nil
}

func (f *FakeAccess) SetOverride(_ context.Context, userID string, v bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Overrides[userID] = v
	return nil
}

func (f *FakeAccess) ClearOverride(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Overrides, userID)
	return nil
}

func (f *FakeAccess) ListOverrides(context.Context) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]bool, len(f.Overrides))
	maps.Copy(out, f.Overrides)
	return out, nil
}

var (
	_ mediarequest.Repository = (*FakeRepository)(nil)
	_ mediarequest.Access     = (*FakeAccess)(nil)
)
