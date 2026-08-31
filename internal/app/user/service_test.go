package user_test

import (
	"context"
	"sync"
	"testing"
	"time"

	appuser "github.com/evey-project/evey/internal/app/user"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAuthSvc() (*appuser.CommandService, *userfakes.FakeUserRepository, *userfakes.FakeProfileRepository, *userfakes.FakePasswordHasher, *sharedfakes.FakeEventBus) {
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	rtRepo := userfakes.NewFakeRefreshTokenRepository()
	roleRepo := userfakes.NewFakeRoleRepository()
	userRoleRepo := userfakes.NewFakeUserRoleRepository(roleRepo)
	hasher := userfakes.NewFakePasswordHasher()
	tokenGen := userfakes.NewFakeTokenGenerator()
	bus := sharedfakes.NewFakeEventBus()
	svc := appuser.NewCommandService(userRepo, profileRepo, rtRepo, roleRepo, userRoleRepo, hasher, tokenGen, bus, 7*24*time.Hour, userfakes.NewFakeQuickConnectRepository())
	return svc, userRepo, profileRepo, hasher, bus
}

func newUserSvc() (*appuser.UserService, *userfakes.FakeUserRepository, *userfakes.FakeProfileRepository, *userfakes.FakePasswordHasher, *sharedfakes.FakeEventBus) {
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	rtRepo := userfakes.NewFakeRefreshTokenRepository()
	hasher := userfakes.NewFakePasswordHasher()
	bus := sharedfakes.NewFakeEventBus()
	svc := appuser.NewUserService(userRepo, profileRepo, rtRepo, hasher, bus, sharedfakes.NewFakeTransactionalUnit())
	return svc, userRepo, profileRepo, hasher, bus
}

func TestSetup(t *testing.T) {
	svc, _, _, _, _ := newAuthSvc()

	u, err := svc.Setup(context.Background(), "admin", "password123", "admin@evey.local")
	require.NoError(t, err)
	assert.Equal(t, "admin", u.Username)
	assert.True(t, u.IsAdmin)

	_, err = svc.Setup(context.Background(), "admin2", "password456", "a2@evey.local")
	assert.ErrorIs(t, err, appuser.ErrSetupDone)
}

func TestLogin_Success(t *testing.T) {
	svc, userRepo, _, hasher, bus := newAuthSvc()

	hash, _ := hasher.Hash("secret")
	_ = userRepo.Create(context.Background(), &domainuser.User{
		ID:           "u1",
		Username:     "alice",
		PasswordHash: hash,
		Enabled:      true,
	})

	access, refresh, err := svc.Login(context.Background(), "alice", "secret")
	require.NoError(t, err)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)

	var loggedIn bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "user.login.succeeded" {
			loggedIn = true
		}
	}
	assert.True(t, loggedIn)
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, userRepo, _, hasher, _ := newAuthSvc()

	hash, _ := hasher.Hash("correct")
	_ = userRepo.Create(context.Background(), &domainuser.User{
		ID:           "u2",
		Username:     "bob",
		PasswordHash: hash,
		Enabled:      true,
	})

	_, _, err := svc.Login(context.Background(), "bob", "wrong")
	assert.ErrorIs(t, err, appuser.ErrInvalidCredentials)
}

func TestLogin_UserNotFound(t *testing.T) {
	svc, _, _, _, _ := newAuthSvc()

	_, _, err := svc.Login(context.Background(), "nobody", "pass")
	assert.ErrorIs(t, err, appuser.ErrInvalidCredentials)
}

func TestCreateProfile(t *testing.T) {
	svc, _, profileRepo, _, bus := newUserSvc()

	p := &domainuser.Profile{
		ID:     "prof-1",
		UserID: "u3",
		Name:   "Kids",
	}
	err := svc.CreateProfile(context.Background(), p)
	require.NoError(t, err)

	profiles, err := profileRepo.GetByUserID(context.Background(), "u3")
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "Kids", profiles[0].Name)

	var profileCreated bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "user.profile.created" {
			profileCreated = true
		}
	}
	assert.True(t, profileCreated)
}

type collidingTokenGen struct {
	mu           sync.Mutex
	inner        *userfakes.FakeTokenGenerator
	fixedToken   string
	collideCount int
	callCount    int
}

func (g *collidingTokenGen) GenerateAccessToken(c domainuser.Claims) (string, error) {
	return g.inner.GenerateAccessToken(c)
}

func (g *collidingTokenGen) GenerateRefreshToken() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.callCount++
	if g.callCount <= g.collideCount {
		return g.fixedToken, nil
	}
	return g.inner.GenerateRefreshToken()
}

func (g *collidingTokenGen) ValidateAccessToken(t string) (*domainuser.Claims, error) {
	return g.inner.ValidateAccessToken(t)
}
func (g *collidingTokenGen) GeneratePINToken(p string, ttl time.Duration) (string, error) {
	return g.inner.GeneratePINToken(p, ttl)
}
func (g *collidingTokenGen) ParsePINToken(raw string) (string, error) {
	return g.inner.ParsePINToken(raw)
}

func TestLogin_DuplicateRefreshTokenRetries(t *testing.T) {
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	rtRepo := userfakes.NewFakeRefreshTokenRepository()
	roleRepo := userfakes.NewFakeRoleRepository()
	userRoleRepo := userfakes.NewFakeUserRoleRepository(roleRepo)
	hasher := userfakes.NewFakePasswordHasher()
	bus := sharedfakes.NewFakeEventBus()

	tokenGen := &collidingTokenGen{
		inner:        userfakes.NewFakeTokenGenerator(),
		fixedToken:   "same-token-every-time",
		collideCount: 1,
	}

	svc := appuser.NewCommandService(userRepo, profileRepo, rtRepo, roleRepo, userRoleRepo, hasher, tokenGen, bus, 7*24*time.Hour, userfakes.NewFakeQuickConnectRepository())

	hash, _ := hasher.Hash("pass")
	_ = userRepo.Create(context.Background(), &domainuser.User{
		ID: "u-dup1", Username: "dup1", PasswordHash: hash, Enabled: true,
	})
	_ = userRepo.Create(context.Background(), &domainuser.User{
		ID: "u-dup2", Username: "dup2", PasswordHash: hash, Enabled: true,
	})

	_, _, err := svc.Login(context.Background(), "dup1", "pass")
	require.NoError(t, err)

	_, refresh, err := svc.Login(context.Background(), "dup2", "pass")
	require.NoError(t, err)
	assert.NotEmpty(t, refresh)
}

func TestLogin_DuplicateRefreshTokenExhausted(t *testing.T) {
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	rtRepo := userfakes.NewFakeRefreshTokenRepository()
	roleRepo := userfakes.NewFakeRoleRepository()
	userRoleRepo := userfakes.NewFakeUserRoleRepository(roleRepo)
	hasher := userfakes.NewFakePasswordHasher()
	bus := sharedfakes.NewFakeEventBus()

	tokenGen := &collidingTokenGen{
		inner:        userfakes.NewFakeTokenGenerator(),
		fixedToken:   "always-same",
		collideCount: 100,
	}

	svc := appuser.NewCommandService(userRepo, profileRepo, rtRepo, roleRepo, userRoleRepo, hasher, tokenGen, bus, 7*24*time.Hour, userfakes.NewFakeQuickConnectRepository())

	hash, _ := hasher.Hash("pass")
	_ = userRepo.Create(context.Background(), &domainuser.User{
		ID: "u-ex1", Username: "ex1", PasswordHash: hash, Enabled: true,
	})
	_ = userRepo.Create(context.Background(), &domainuser.User{
		ID: "u-ex2", Username: "ex2", PasswordHash: hash, Enabled: true,
	})

	_, _, err := svc.Login(context.Background(), "ex1", "pass")
	require.NoError(t, err)

	_, _, err = svc.Login(context.Background(), "ex2", "pass")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate hash after retries")
}

func TestLogin_ConcurrentSameUser(t *testing.T) {
	svc, userRepo, _, hasher, _ := newAuthSvc()

	hash, _ := hasher.Hash("pass")
	_ = userRepo.Create(context.Background(), &domainuser.User{
		ID: "u-conc", Username: "concurrent", PasswordHash: hash, Enabled: true,
	})

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, errs[idx] = svc.Login(context.Background(), "concurrent", "pass")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "login %d failed", i)
	}
}

func TestRegister_HashedPassword(t *testing.T) {
	svc, userRepo, _, _, _ := newUserSvc()

	err := svc.CreateUser(context.Background(), true, &domainuser.User{
		ID:       "u4",
		Username: "dave",
		Email:    "dave@evey.local",
		Enabled:  true,
	}, "mypassword")
	require.NoError(t, err)

	stored, err := userRepo.GetByUsername(context.Background(), "dave")
	require.NoError(t, err)
	assert.NotEqual(t, "mypassword", stored.PasswordHash)
	assert.NotEmpty(t, stored.PasswordHash)
}

func TestUpdateProfile_BitrateLimit(t *testing.T) {
	svc, _, profileRepo, _, _ := newUserSvc()

	_ = profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:     "prof-br",
		UserID: "u-br",
		Name:   "Main",
	})

	bitrate := 8000
	err := svc.UpdateProfile(context.Background(), &domainuser.Profile{
		ID:             "prof-br",
		Name:           "Main",
		MaxBitrateKbps: &bitrate,
	})
	require.NoError(t, err)

	updated, err := profileRepo.GetByID(context.Background(), "prof-br")
	require.NoError(t, err)
	require.NotNil(t, updated.MaxBitrateKbps)
	assert.Equal(t, 8000, *updated.MaxBitrateKbps)

	err = svc.UpdateProfile(context.Background(), &domainuser.Profile{
		ID:             "prof-br",
		Name:           "Main",
		MaxBitrateKbps: nil,
	})
	require.NoError(t, err)

	updated, err = profileRepo.GetByID(context.Background(), "prof-br")
	require.NoError(t, err)
	assert.Nil(t, updated.MaxBitrateKbps)
}

func (g *collidingTokenGen) DescribeRefreshToken(raw string) string {
	return g.inner.DescribeRefreshToken(raw)
}
