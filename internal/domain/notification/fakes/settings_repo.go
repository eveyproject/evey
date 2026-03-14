package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

type FakeSettingsRepository struct {
	mu        sync.Mutex
	byProfile map[string]*notification.Settings
	defaults  *notification.Settings
	Err       error
}

func NewFakeSettingsRepository() *FakeSettingsRepository {
	return &FakeSettingsRepository{byProfile: make(map[string]*notification.Settings)}
}

func (r *FakeSettingsRepository) GetByProfile(_ context.Context, profileID string) (*notification.Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
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

func (r *FakeSettingsRepository) GetByProfiles(ctx context.Context, profileIDs []string) (map[string]*notification.Settings, error) {
	result := make(map[string]*notification.Settings, len(profileIDs))
	for _, id := range profileIDs {
		s, err := r.GetByProfile(ctx, id)
		if err != nil {
			return nil, err
		}
		result[id] = s
	}
	return result, nil
}

func (r *FakeSettingsRepository) Upsert(_ context.Context, s *notification.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.byProfile[s.ProfileID] = s
	return nil
}

func (r *FakeSettingsRepository) GetGlobalDefaults(_ context.Context) (*notification.Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	if r.defaults == nil {
		return &notification.Settings{MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}, nil
	}
	return r.defaults, nil
}

func (r *FakeSettingsRepository) UpsertGlobalDefaults(_ context.Context, s *notification.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.defaults = s
	return nil
}
