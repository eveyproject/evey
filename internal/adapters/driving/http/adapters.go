package httpapi

import (
	"context"

	domainshared "github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// UserMaxStreamsAdapter adapts a UserRepository to the UserMaxStreamsGetter interface.
type UserMaxStreamsAdapter struct {
	repo interface {
		GetByID(ctx context.Context, id domainshared.UserID) (*domainuser.User, error)
	}
}

func (a *UserMaxStreamsAdapter) GetMaxStreams(ctx context.Context, userID string) *int {
	u, err := a.repo.GetByID(ctx, domainshared.UserID(userID))
	if err != nil {
		return nil
	}
	return u.MaxStreams
}

func NewUserMaxStreamsAdapter(repo interface {
	GetByID(ctx context.Context, id domainshared.UserID) (*domainuser.User, error)
}) *UserMaxStreamsAdapter {
	return &UserMaxStreamsAdapter{repo: repo}
}

// UserBitrateAdapter adapts a query service to the UserBitrateGetter interface.
type UserBitrateAdapter struct {
	svc interface {
		GetUserBitrateLimit(ctx context.Context, userID string) (int, error)
	}
}

func (a *UserBitrateAdapter) GetUserBitrateLimit(ctx context.Context, userID string) (int, error) {
	return a.svc.GetUserBitrateLimit(ctx, userID)
}

func NewUserBitrateAdapter(svc interface {
	GetUserBitrateLimit(ctx context.Context, userID string) (int, error)
}) *UserBitrateAdapter {
	return &UserBitrateAdapter{svc: svc}
}

// ProfilePrefsAdapter adapts a profile-getter service to ProfilePrefsGetter.
type ProfilePrefsAdapter struct {
	svc interface {
		GetProfileByID(ctx context.Context, profileID string) (*domainuser.Profile, error)
	}
}

func (a *ProfilePrefsAdapter) GetProfilePreferences(ctx context.Context, profileID string) (map[string]string, error) {
	p, err := a.svc.GetProfileByID(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return p.Preferences, nil
}

// GetProfileAudioPrefs returns the profile-level audio language fields that
// live in dedicated columns (not the JSONB preferences map). This allows
// loadProfilePrefs to populate TranscodingPreferences.AudioLanguages and
// .Locale without a second database round-trip (same GetProfileByID call path
// via the svc cache).
func (a *ProfilePrefsAdapter) GetProfileAudioPrefs(ctx context.Context, profileID string) (audioLanguages []string, locale string, err error) {
	p, err := a.svc.GetProfileByID(ctx, profileID)
	if err != nil {
		return nil, "", err
	}
	return p.AudioLanguages, p.Locale, nil
}

func NewProfilePrefsAdapter(svc interface {
	GetProfileByID(ctx context.Context, profileID string) (*domainuser.Profile, error)
}) *ProfilePrefsAdapter {
	return &ProfilePrefsAdapter{svc: svc}
}

// ServerConfigStore adapts ServerConfigRepository to the remoteAccessPersister interface.
type ServerConfigStore struct {
	repo domainshared.ServerConfigRepository
}

func (s *ServerConfigStore) Get(ctx context.Context, key string) (string, error) {
	e, err := s.repo.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return e.Value, nil
}

func (s *ServerConfigStore) Set(ctx context.Context, key, value string) error {
	return s.repo.Set(ctx, key, value)
}

func (s *ServerConfigStore) Delete(ctx context.Context, key string) error {
	return s.repo.Delete(ctx, key)
}

func NewServerConfigStore(repo domainshared.ServerConfigRepository) *ServerConfigStore {
	return &ServerConfigStore{repo: repo}
}
