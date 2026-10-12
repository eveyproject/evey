package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeSettingsRepository struct {
	mu        sync.Mutex
	byProfile map[shared.ProfileID]*notification.Settings
	defaults  *notification.Settings
	Err       error
}

func NewFakeSettingsRepository() *FakeSettingsRepository {
	return &FakeSettingsRepository{byProfile: make(map[shared.ProfileID]*notification.Settings)}
}

func (r *FakeSettingsRepository) GetByProfiles(_ context.Context, profileIDs []shared.ProfileID) (map[shared.ProfileID]*notification.Settings, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[shared.ProfileID]*notification.Settings, len(profileIDs))
	for _, id := range profileIDs {
		if s, ok := r.byProfile[id]; ok {
			result[id] = s
		} else if r.defaults != nil {
			cp := *r.defaults
			cp.ProfileID = id
			result[id] = &cp
		} else {
			result[id] = &notification.Settings{ProfileID: id, MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}
		}
	}
	return result, nil
}

func (r *FakeSettingsRepository) GetByProfile(_ context.Context, profileID shared.ProfileID) (*notification.Settings, error) {
	if r.Err != nil {
		return nil, r.Err
	}
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

func (r *FakeSettingsRepository) Upsert(_ context.Context, s *notification.Settings) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.byProfile[s.ProfileID] = s
	return nil
}

func (r *FakeSettingsRepository) GetGlobalDefaults(_ context.Context) (*notification.Settings, error) {
	if r.Err != nil {
		return nil, r.Err
	}
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
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.defaults = s
	return nil
}
