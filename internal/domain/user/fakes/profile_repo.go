package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

type FakeProfileRepository struct {
	mu    sync.RWMutex
	items map[string]*user.Profile
	Err   error
}

func NewFakeProfileRepository() *FakeProfileRepository {
	return &FakeProfileRepository{items: make(map[string]*user.Profile)}
}

func (r *FakeProfileRepository) Create(_ context.Context, p *user.Profile) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(p.ID)] = p
	return nil
}

// CreateWithinCap mirrors the repo: count + insert under the same lock so the
// cap holds atomically. maxProfiles <= 0 means unlimited.
func (r *FakeProfileRepository) CreateWithinCap(_ context.Context, p *user.Profile, maxProfiles int) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if maxProfiles > 0 {
		n := 0
		for _, existing := range r.items {
			if existing.UserID == p.UserID {
				n++
			}
		}
		if n >= maxProfiles {
			return user.ErrProfileCapReached
		}
	}
	r.items[string(p.ID)] = p
	return nil
}

func (r *FakeProfileRepository) GetByID(_ context.Context, id shared.ProfileID) (*user.Profile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	p, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakeProfileRepository) GetByUserID(_ context.Context, userID shared.UserID) ([]*user.Profile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*user.Profile
	for _, p := range r.items {
		if p.UserID == userID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *FakeProfileRepository) Update(_ context.Context, p *user.Profile) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[string(p.ID)]; !ok {
		return ErrNotFound
	}
	r.items[string(p.ID)] = p
	return nil
}

func (r *FakeProfileRepository) GetAll(_ context.Context) ([]*user.Profile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	out := make([]*user.Profile, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, p)
	}
	return out, nil
}

func (r *FakeProfileRepository) SetDefault(_ context.Context, userID shared.UserID, profileID shared.ProfileID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	target, ok := r.items[string(profileID)]
	if !ok || target.UserID != userID {
		return ErrNotFound
	}
	for _, p := range r.items {
		if p.UserID == userID {
			p.IsDefault = false
		}
	}
	target.IsDefault = true
	return nil
}

func (r *FakeProfileRepository) Delete(_ context.Context, id shared.ProfileID) error {
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
