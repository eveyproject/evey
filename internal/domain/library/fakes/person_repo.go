package fakes

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakePersonRepository struct {
	mu    sync.RWMutex
	items map[string]*library.Person
	Err   error
	// UpdateCalls counts Update invocations, so callers can assert a bio-check
	// negative cache (or lack thereof) actually persisted.
	UpdateCalls atomic.Int64
}

func NewFakePersonRepository() *FakePersonRepository {
	return &FakePersonRepository{items: make(map[string]*library.Person)}
}

func (r *FakePersonRepository) Create(_ context.Context, person *library.Person) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	// Mirrors the Postgres identity: an existing row with the same provider
	// id, or for an id-less person the same name among the id-less rows, is
	// the person; the caller gets its id back.
	if existing := r.identityMatchLocked(person); existing != nil && existing.ID != person.ID {
		person.ID = existing.ID
		return nil
	}
	r.items[person.ID] = person
	return nil
}

func (r *FakePersonRepository) ClaimProviderID(_ context.Context, personID, provider, id string) (string, error) {
	if r.Err != nil {
		return "", r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	owner := ""
	for _, p := range r.items {
		if p.ProviderIDs[provider] == id && (owner == "" || p.ID < owner) {
			owner = p.ID
		}
	}
	if owner != "" {
		return owner, nil
	}
	p, ok := r.items[personID]
	if !ok {
		return "", ErrNotFound
	}
	if p.ProviderIDs == nil {
		p.ProviderIDs = map[string]string{}
	}
	p.ProviderIDs[provider] = id
	return personID, nil
}

func (r *FakePersonRepository) identityMatchLocked(person *library.Person) *library.Person {
	tmdb := person.ProviderIDs["tmdb"]
	var best *library.Person
	for _, p := range r.items {
		var match bool
		if tmdb != "" {
			match = p.ProviderIDs["tmdb"] == tmdb
		} else {
			match = p.ProviderIDs["tmdb"] == "" && p.Name == person.Name
		}
		if match && (best == nil || p.ID < best.ID) {
			best = p
		}
	}
	return best
}

// preferredByName picks, among homonyms, the row a name may stand for: the
// one without a provider id, then the lowest id, like the Postgres ORDER BY.
func preferredByName(cur, cand *library.Person) bool {
	if cur == nil {
		return true
	}
	curHasID, candHasID := cur.ProviderIDs["tmdb"] != "", cand.ProviderIDs["tmdb"] != ""
	if curHasID != candHasID {
		return curHasID
	}
	return cand.ID < cur.ID
}

func (r *FakePersonRepository) Update(_ context.Context, person *library.Person) error {
	r.UpdateCalls.Add(1)
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[person.ID] = person
	return nil
}

func (r *FakePersonRepository) GetByID(_ context.Context, id string) (*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakePersonRepository) GetByName(_ context.Context, name string) (*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var best *library.Person
	for _, p := range r.items {
		if p.Name == name && preferredByName(best, p) {
			best = p
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	return best, nil
}

func (r *FakePersonRepository) GetByProviderID(_ context.Context, provider, id string) (*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *library.Person
	for _, p := range r.items {
		if id != "" && p.ProviderIDs[provider] == id && (best == nil || p.ID < best.ID) {
			best = p
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	return best, nil
}

func (r *FakePersonRepository) GetByNamesBatch(_ context.Context, names []string) (map[string]*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[n] = struct{}{}
	}
	out := make(map[string]*library.Person, len(names))
	for _, p := range r.items {
		if _, ok := want[p.Name]; ok && preferredByName(out[p.Name], p) {
			out[p.Name] = p
		}
	}
	return out, nil
}

func (r *FakePersonRepository) List(_ context.Context) ([]*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make([]*library.Person, 0, len(r.items))
	for _, p := range r.items {
		result = append(result, p)
	}
	return result, nil
}
