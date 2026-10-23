package fakes

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeTrustedPublisherRepository struct {
	mu    sync.RWMutex
	items map[string]plugin.TrustedPublisher
	Err   error
}

func NewFakeTrustedPublisherRepository() *FakeTrustedPublisherRepository {
	return &FakeTrustedPublisherRepository{items: make(map[string]plugin.TrustedPublisher)}
}

func (r *FakeTrustedPublisherRepository) List(_ context.Context) ([]plugin.TrustedPublisher, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]plugin.TrustedPublisher, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out, nil
}

func (r *FakeTrustedPublisherRepository) Get(_ context.Context, fingerprint string) (*plugin.TrustedPublisher, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[fingerprint]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return &p, nil
}

func (r *FakeTrustedPublisherRepository) Upsert(_ context.Context, p plugin.TrustedPublisher) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.items[p.Fingerprint]; ok {
		p.AddedAt = old.AddedAt
		p.Seeded = p.Seeded || old.Seeded
	} else if p.AddedAt.IsZero() {
		p.AddedAt = time.Now().UTC()
	}
	r.items[p.Fingerprint] = p
	return nil
}

func (r *FakeTrustedPublisherRepository) Delete(_ context.Context, fingerprint string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[fingerprint]
	if !ok {
		return shared.ErrNotFound
	}
	if p.Seeded {
		return plugin.ErrSeededPublisher
	}
	delete(r.items, fingerprint)
	return nil
}

func (r *FakeTrustedPublisherRepository) PruneSeeded(_ context.Context, keep []string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for fp, p := range r.items {
		if p.Seeded && !slices.Contains(keep, fp) {
			delete(r.items, fp)
		}
	}
	return nil
}

var _ plugin.TrustedPublisherRepository = (*FakeTrustedPublisherRepository)(nil)
