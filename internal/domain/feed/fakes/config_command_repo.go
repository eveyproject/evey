package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/feed"
)

type FakeConfigCommandRepo struct {
	mu             sync.Mutex
	AdminConfigs   map[feed.Type]*feed.AdminConfig
	ProfileConfigs map[string]map[feed.Type]*feed.ProfileConfig
	Err            error
}

func NewFakeConfigCommandRepo() *FakeConfigCommandRepo {
	return &FakeConfigCommandRepo{
		AdminConfigs:   make(map[feed.Type]*feed.AdminConfig),
		ProfileConfigs: make(map[string]map[feed.Type]*feed.ProfileConfig),
	}
}

func (r *FakeConfigCommandRepo) UpsertAdminConfig(_ context.Context, cfg *feed.AdminConfig) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.AdminConfigs[cfg.FeedType] = cfg
	return nil
}

func (r *FakeConfigCommandRepo) BatchUpsertAdminConfig(_ context.Context, cfgs []*feed.AdminConfig) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cfg := range cfgs {
		r.AdminConfigs[cfg.FeedType] = cfg
	}
	return nil
}

func (r *FakeConfigCommandRepo) DeleteAdminConfigByType(_ context.Context, feedType feed.Type) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.AdminConfigs, feedType)
	for pid, m := range r.ProfileConfigs {
		delete(m, feedType)
		if len(m) == 0 {
			delete(r.ProfileConfigs, pid)
		}
	}
	return nil
}

func (r *FakeConfigCommandRepo) UpsertProfileConfig(_ context.Context, cfg *feed.ProfileConfig) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.ProfileConfigs[string(cfg.ProfileID)] == nil {
		r.ProfileConfigs[string(cfg.ProfileID)] = make(map[feed.Type]*feed.ProfileConfig)
	}
	r.ProfileConfigs[string(cfg.ProfileID)][cfg.FeedType] = cfg
	return nil
}

func (r *FakeConfigCommandRepo) DeleteProfileConfig(_ context.Context, profileID string, feedType feed.Type) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if m, ok := r.ProfileConfigs[profileID]; ok {
		delete(m, feedType)
	}
	return nil
}
