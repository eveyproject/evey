package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/feed"
)

type FakeConfigQueryRepo struct {
	mu             sync.Mutex
	AdminConfigs   []*feed.AdminConfig
	ProfileConfigs map[string][]*feed.ProfileConfig
}

func NewFakeConfigQueryRepo() *FakeConfigQueryRepo {
	return &FakeConfigQueryRepo{
		ProfileConfigs: make(map[string][]*feed.ProfileConfig),
	}
}

func (r *FakeConfigQueryRepo) GetAdminConfig(_ context.Context) ([]*feed.AdminConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.AdminConfigs, nil
}

func (r *FakeConfigQueryRepo) GetProfileConfig(_ context.Context, profileID string) ([]*feed.ProfileConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ProfileConfigs[profileID], nil
}
