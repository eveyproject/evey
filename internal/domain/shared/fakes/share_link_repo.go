package fakes

import (
	"context"
	"sync"

	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeShareLinkRepo struct {
	mu    sync.RWMutex
	links []*shared.ShareLink
	Err   error
	// DeleteErr fails only Delete, leaving lookups working - used to exercise a
	// revoke that fails after the existence/ownership checks have passed.
	DeleteErr error
}

func NewFakeShareLinkRepo() *FakeShareLinkRepo {
	return &FakeShareLinkRepo{}
}

func (r *FakeShareLinkRepo) Create(_ context.Context, link *shared.ShareLink) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.links = append(r.links, link)
	return nil
}

func (r *FakeShareLinkRepo) GetByID(_ context.Context, id string) (*shared.ShareLink, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, l := range r.links {
		if l.ID == id {
			return l, nil
		}
	}
	return nil, nil
}

func (r *FakeShareLinkRepo) GetByToken(_ context.Context, token string) (*shared.ShareLink, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, l := range r.links {
		if l.Token == token {
			return l, nil
		}
	}
	return nil, shared.ErrNotFound
}

func (r *FakeShareLinkRepo) ListByUser(_ context.Context, userID string) ([]*shared.ShareLink, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*shared.ShareLink
	for _, l := range r.links {
		if l.CreatedBy == shared.UserID(userID) {
			result = append(result, l)
		}
	}
	return result, nil
}

func (r *FakeShareLinkRepo) ListAll(_ context.Context) ([]*shared.ShareLink, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.links, nil
}

func (r *FakeShareLinkRepo) ConsumeView(_ context.Context, token string) (*shared.ShareLink, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.links {
		if l.Token == token {
			if l.IsExpired() || l.IsViewLimitReached() {
				return nil, shared.ErrNotFound
			}
			l.ViewCount++
			return l, nil
		}
	}
	return nil, shared.ErrNotFound
}

func (r *FakeShareLinkRepo) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	if r.DeleteErr != nil {
		return r.DeleteErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, l := range r.links {
		if l.ID == id {
			r.links = append(r.links[:i], r.links[i+1:]...)
			return nil
		}
	}
	return shared.ErrNotFound
}

func (r *FakeShareLinkRepo) DeleteExpired(_ context.Context) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var kept []*shared.ShareLink
	deleted := 0
	for _, l := range r.links {
		if l.IsExpired() {
			deleted++
		} else {
			kept = append(kept, l)
		}
	}
	r.links = kept
	return deleted, nil
}

func (r *FakeShareLinkRepo) Seed(link *shared.ShareLink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.links = append(r.links, link)
}
