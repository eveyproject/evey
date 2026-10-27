package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/user"
)

// A non-kids profile with parental control enabled and a PIN is restricted
// just like a kids one: the gates must key on IsParentalRestricted, not IsKids.

func seedLockedProfile(t *testing.T, env *testUserEnv) {
	t.Helper()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "family", Email: "family@example.com"}, "pw-1")
	age := 12
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-1", UserID: "usr-1", Name: "Family", IsDefault: true,
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &age, PINHash: "hashed:1111"},
	}))
}

func storedMaxAge(t *testing.T, env *testUserEnv) *int {
	t.Helper()
	p, err := env.profileRepo.GetByID(context.Background(), "prf-1")
	require.NoError(t, err)
	if p.Parental == nil {
		return nil
	}
	return p.Parental.MaxAge
}

func TestUpdateProfile_LockedProfileCannotLiftItsOwnAgeCapWithoutPIN(t *testing.T) {
	env := setupUserHandler()
	seedLockedProfile(t, env)
	ctx := userCtx("usr-1", "prf-1")
	lift := int32(-1)

	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", MaxAge: &lift})
	require.Error(t, err)
	require.NotNil(t, storedMaxAge(t, env))
	assert.Equal(t, 12, *storedMaxAge(t, env))

	raise := int32(18)
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", MaxAge: &raise})
	require.Error(t, err)
	assert.Equal(t, 12, *storedMaxAge(t, env))

	wrong := "0000"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", MaxAge: &raise, CurrentPin: &wrong})
	assertCode(t, err, codes.PermissionDenied)
	assert.Equal(t, 12, *storedMaxAge(t, env))

	right := "1111"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", MaxAge: &raise, CurrentPin: &right})
	require.NoError(t, err)
	assert.Equal(t, 18, *storedMaxAge(t, env))

	// Tightening needs no proof.
	lower := int32(10)
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", MaxAge: &lower})
	require.NoError(t, err)
	assert.Equal(t, 10, *storedMaxAge(t, env))
}

func TestCreateProfile_RefusedFromLockedProfile(t *testing.T) {
	env := setupUserHandler()
	seedLockedProfile(t, env)
	pin := "4321"
	_, err := env.handler.CreateProfile(userCtx("usr-1", "prf-1"), &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "Free", Pin: &pin})
	assertCode(t, err, codes.PermissionDenied)
	profiles, lerr := env.profileRepo.GetByUserID(context.Background(), "usr-1")
	require.NoError(t, lerr)
	assert.Len(t, profiles, 1)
}

func TestAuthorizeQuickConnect_RefusedFromLockedProfile(t *testing.T) {
	env := setupUserHandler()
	seedLockedProfile(t, env)
	initResp, err := env.handler.InitiateQuickConnect(context.Background(), &eveyv1.InitiateQuickConnectRequest{})
	require.NoError(t, err)
	_, err = env.handler.AuthorizeQuickConnect(userCtx("usr-1", "prf-1"), &eveyv1.AuthorizeQuickConnectRequest{Code: initResp.Code})
	assertCode(t, err, codes.PermissionDenied)
}
