package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/feed"
)

type FakeConfigQueryRepo struct {
	mu             sync.Mutex
	AdminConfigs   []*feed.AdminConfig
	ProfileConfigs map[string][]*feed.ProfileConfig
	Err            error
}

func NewFakeConfigQueryRepo() *FakeConfigQueryRepo {
	return &FakeConfigQueryRepo{
		ProfileConfigs: make(map[string][]*feed.ProfileConfig),
	}
}

func (r *FakeConfigQueryRepo) GetAdminConfig(_ context.Context) ([]*feed.AdminConfig, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return r.AdminConfigs, nil
}

func (r *FakeConfigQueryRepo) GetProfileConfig(_ context.Context, profileID string) ([]*feed.ProfileConfig, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return r.ProfileConfigs[profileID], nil
}
