package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// An age cap set to the one already in force, or to 0, tightens nothing:
// neither asks for the PIN. 0 is a cap like any other, not "no cap", which
// only a negative value means (and that one does ask).
func TestUpdateProfile_AnAgeCapAtOrBelowTheCurrentOneNeedsNoPIN(t *testing.T) {
	env := setupUserHandler()
	seedLockedProfile(t, env)
	ctx := userCtx("usr-1", "prf-1")

	same := int32(12)
	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", MaxAge: &same})
	require.NoError(t, err, "the cap already in force asked for the PIN")
	require.NotNil(t, storedMaxAge(t, env))
	assert.Equal(t, 12, *storedMaxAge(t, env))

	zero := int32(0)
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", MaxAge: &zero})
	require.NoError(t, err, "a cap of 0 tightens and asked for the PIN")
	require.NotNil(t, storedMaxAge(t, env), "a cap of 0 was taken for no cap")
	assert.Equal(t, 0, *storedMaxAge(t, env))
}

// A profile created with a cap of 0 or a rating keeps them.
func TestCreateProfile_KeepsAZeroAgeCapAndTheRating(t *testing.T) {
	env := setupUserHandler()
	seedAdultProfile(t, env, "prf-1", "usr-1")
	zero := int32(0)
	resp, err := env.handler.CreateProfile(userCtx("usr-1", "prf-1"), &eveyv1.CreateProfileRequest{
		UserId: "usr-1", Name: "Little", MaxAge: &zero, MaxRating: "PG",
	})
	require.NoError(t, err)
	p, err := env.profileRepo.GetByID(context.Background(), shared.ProfileID(resp.Id))
	require.NoError(t, err)
	require.NotNil(t, p.Parental)
	require.NotNil(t, p.Parental.MaxAge, "a cap of 0 was dropped at creation")
	assert.Equal(t, 0, *p.Parental.MaxAge)
	require.NotNil(t, p.MaxRating)
	assert.Equal(t, "PG", *p.MaxRating)
}

// An update that does not mention the rating keeps the stored one.
func TestUpdateProfile_ARatingNotSentIsKept(t *testing.T) {
	env := setupUserHandler()
	pg := "PG-13"
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "Default", MaxRating: &pg}))

	name := "Renamed"
	_, err := env.handler.UpdateProfile(userCtx("usr-1", "prf-1"), &eveyv1.UpdateProfileRequest{Id: "prf-1", Name: &name})
	require.NoError(t, err)
	p, err := env.profileRepo.GetByID(context.Background(), "prf-1")
	require.NoError(t, err)
	require.NotNil(t, p.MaxRating, "a rename cleared the rating")
	assert.Equal(t, "PG-13", *p.MaxRating)
}

// A limit of 0 is a value, only a negative one is refused.
func TestUpdateUser_LimitsAtZeroAreAccepted(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "adm-1", Username: "admin", IsAdmin: true}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-2", Username: "viewer"}, "pass12345678")
	ctx := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "adm-1", Username: "admin", IsAdmin: true, ProfileID: "prf-1"})

	zero, neg := int32(0), int32(-1)
	_, err := env.handler.UpdateUser(ctx, &eveyv1.UpdateUserRequest{Id: "usr-2", MaxProfiles: &zero, MaxDevicesPerProfile: &zero})
	require.NoError(t, err)
	u, err := env.userRepo.GetByID(context.Background(), "usr-2")
	require.NoError(t, err)
	require.NotNil(t, u.MaxProfiles)
	require.NotNil(t, u.MaxDevicesPerProfile)
	assert.Equal(t, 0, *u.MaxProfiles)
	assert.Equal(t, 0, *u.MaxDevicesPerProfile)

	_, err = env.handler.UpdateUser(ctx, &eveyv1.UpdateUserRequest{Id: "usr-2", MaxProfiles: &neg})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = env.handler.UpdateUser(ctx, &eveyv1.UpdateUserRequest{Id: "usr-2", MaxDevicesPerProfile: &neg})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
