package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

type FakeSettingsRepository struct {
	mu       sync.Mutex
	byProfile map[string]*notification.Settings
	defaults  *notification.Settings
}

func NewFakeSettingsRepository() *FakeSettingsRepository {
	return &FakeSettingsRepository{byProfile: make(map[string]*notification.Settings)}
}

func (r *FakeSettingsRepository) GetByProfile(_ context.Context, profileID string) (*notification.Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byProfile[profileID]; ok {
		return s, nil
	}
	if r.defaults != nil {
		cp := *r.defaults
		cp.ProfileID = profileID
		return &cp, nil
	}
	return &notification.Settings{ProfileID: profileID, MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}, nil
}

func (r *FakeSettingsRepository) Upsert(_ context.Context, s *notification.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byProfile[s.ProfileID] = s
	return nil
}

func (r *FakeSettingsRepository) GetGlobalDefaults(_ context.Context) (*notification.Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.defaults == nil {
		return &notification.Settings{MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}, nil
	}
	return r.defaults, nil
}

func (r *FakeSettingsRepository) UpsertGlobalDefaults(_ context.Context, s *notification.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaults = s
	return nil
}
