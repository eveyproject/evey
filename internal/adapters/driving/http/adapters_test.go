package httpapi

import (
	"context"
	"errors"
	"testing"

	domainshared "github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubUserRepo struct {
	user *domainuser.User
	err  error
}

func (r *stubUserRepo) GetByID(_ context.Context, _ domainshared.UserID) (*domainuser.User, error) {
	return r.user, r.err
}

func TestUserMaxStreamsAdapter_HappyPath(t *testing.T) {
	t.Parallel()
	n := 3
	a := NewUserMaxStreamsAdapter(&stubUserRepo{user: &domainuser.User{MaxStreams: &n}})
	got := a.GetMaxStreams(context.Background(), "u1")
	require.NotNil(t, got)
	assert.Equal(t, 3, *got)
}

func TestUserMaxStreamsAdapter_RepoError_ReturnsNil(t *testing.T) {
	t.Parallel()
	a := NewUserMaxStreamsAdapter(&stubUserRepo{err: errors.New("db down")})
	got := a.GetMaxStreams(context.Background(), "u1")
	assert.Nil(t, got)
}

type stubBitrateSvc struct {
	kbps int
	err  error
}

func (s *stubBitrateSvc) GetUserBitrateLimit(_ context.Context, _ string) (int, error) {
	return s.kbps, s.err
}

func TestUserBitrateAdapter_Delegates(t *testing.T) {
	t.Parallel()
	a := NewUserBitrateAdapter(&stubBitrateSvc{kbps: 5000})
	got, err := a.GetUserBitrateLimit(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, 5000, got)
}

type stubProfileSvc struct {
	profile *domainuser.Profile
	err     error
}

func (s *stubProfileSvc) GetProfileByID(_ context.Context, _ string) (*domainuser.Profile, error) {
	return s.profile, s.err
}

func TestProfilePrefsAdapter_HappyPath(t *testing.T) {
	t.Parallel()
	a := NewProfilePrefsAdapter(&stubProfileSvc{
		profile: &domainuser.Profile{Preferences: map[string]string{"theme": "dark"}},
	})
	prefs, err := a.GetProfilePreferences(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, "dark", prefs["theme"])
}

func TestProfilePrefsAdapter_SvcError_Propagates(t *testing.T) {
	t.Parallel()
	a := NewProfilePrefsAdapter(&stubProfileSvc{err: errors.New("not found")})
	_, err := a.GetProfilePreferences(context.Background(), "p1")
	require.Error(t, err)
}
