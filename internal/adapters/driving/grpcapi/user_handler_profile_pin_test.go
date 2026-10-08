package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/user"
)

// The gRPC UpdateProfile used to take a new PIN on the access token alone,
// while the parental HTTP route demanded the current PIN (or the account
// password for a first one). A stolen token could rotate the guardian PIN
// and walk a kids profile out of its restrictions. Same rule on both routes.

func seedPINProfile(t *testing.T, env *testUserEnv, pinHash string) {
	t.Helper()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "parent", Email: "parent@example.com"}, "pw-1")
	p := &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "Parent"}
	if pinHash != "" {
		p.Parental = &user.ParentalSettings{PINHash: pinHash}
	}
	require.NoError(t, env.profileRepo.Create(context.Background(), p))
}

func storedPINHash(t *testing.T, env *testUserEnv) string {
	t.Helper()
	p, err := env.profileRepo.GetByID(context.Background(), "prf-1")
	require.NoError(t, err)
	if p.Parental == nil {
		return ""
	}
	return p.Parental.PINHash
}

func TestUpdateProfile_RotatingAPINNeedsTheCurrentOne(t *testing.T) {
	env := setupUserHandler()
	seedPINProfile(t, env, "hashed:1111")
	ctx := userCtx("usr-1", "prf-1")
	newPIN := "2222"

	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &newPIN})
	assertCode(t, err, codes.InvalidArgument)
	assert.Equal(t, "hashed:1111", storedPINHash(t, env), "no proof, no change")

	wrong := "9999"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &newPIN, CurrentPin: &wrong})
	assertCode(t, err, codes.PermissionDenied)
	assert.Equal(t, "hashed:1111", storedPINHash(t, env), "a wrong PIN changes nothing")

	right := "1111"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &newPIN, CurrentPin: &right})
	require.NoError(t, err)
	assert.Equal(t, "hashed:2222", storedPINHash(t, env))
}

func TestUpdateProfile_ClearingAPINNeedsTheCurrentOne(t *testing.T) {
	env := setupUserHandler()
	seedPINProfile(t, env, "hashed:1111")
	ctx := userCtx("usr-1", "prf-1")
	clear := ""

	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &clear})
	assertCode(t, err, codes.InvalidArgument)
	assert.Equal(t, "hashed:1111", storedPINHash(t, env))

	right := "1111"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &clear, CurrentPin: &right})
	require.NoError(t, err)
	assert.Empty(t, storedPINHash(t, env))
}

func TestUpdateProfile_FirstPINNeedsTheAccountPassword(t *testing.T) {
	env := setupUserHandler()
	seedPINProfile(t, env, "")
	ctx := userCtx("usr-1", "prf-1")
	pin := "1234"

	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &pin})
	assertCode(t, err, codes.InvalidArgument)
	assert.Empty(t, storedPINHash(t, env), "a token alone does not set a PIN")

	wrong := "not-it"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &pin, CurrentPassword: &wrong})
	assertCode(t, err, codes.PermissionDenied)
	assert.Empty(t, storedPINHash(t, env))

	right := "pw-1"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &pin, CurrentPassword: &right})
	require.NoError(t, err)
	assert.Equal(t, "hashed:1234", storedPINHash(t, env))

	// The PIN is not the only field: a request that leaves it alone needs no
	// proof at all, and an empty PIN on a profile without one is a no-op.
	name, none := "Renamed", ""
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Name: &name})
	require.NoError(t, err)
	env2 := setupUserHandler()
	seedPINProfile(t, env2, "")
	_, err = env2.handler.UpdateProfile(userCtx("usr-1", "prf-1"), &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &none})
	require.NoError(t, err)
}

// An admin editing another user's profile does not know that user's PIN: they
// prove their own password instead, for a rotation as for a first PIN.
func TestUpdateProfile_AdminOnAnotherUsersProfileProvesTheirPassword(t *testing.T) {
	env := setupUserHandler()
	seedPINProfile(t, env, "hashed:1111")
	seedUser(t, env, &user.User{ID: "adm-1", Username: "admin", Email: "admin@example.com", IsAdmin: true}, "adm-pw")
	// The parental gate loads the caller's own profile to check it is not a
	// kids one; the admin acts from their default profile.
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-adm", UserID: "adm-1", Name: "Admin", IsDefault: true}))
	ctx := ctxWithClaims("adm-1", "prf-adm", true)
	newPIN := "2222"

	usersPIN := "1111"
	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &newPIN, CurrentPin: &usersPIN})
	assertCode(t, err, codes.InvalidArgument)
	assert.Equal(t, "hashed:1111", storedPINHash(t, env), "the user's PIN is not the admin's proof")

	pw := "adm-pw"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &newPIN, CurrentPassword: &pw})
	require.NoError(t, err)
	assert.Equal(t, "hashed:2222", storedPINHash(t, env))
}

// 15 wrong PINs in a row and the profile's budget is spent: the 16th attempt
// is refused before it is even compared, right PIN or not.
func TestUpdateProfile_CurrentPINAttemptsAreRateLimited(t *testing.T) {
	env := setupUserHandler()
	seedPINProfile(t, env, "hashed:1111")
	ctx := userCtx("usr-1", "prf-1")
	newPIN, wrong := "2222", "0000"

	for range 15 {
		_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &newPIN, CurrentPin: &wrong})
		assertCode(t, err, codes.PermissionDenied)
	}
	right := "1111"
	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &newPIN, CurrentPin: &right})
	assertCode(t, err, codes.ResourceExhausted)
	assert.Equal(t, "hashed:1111", storedPINHash(t, env))
}

func TestProfileToProto_HasPin(t *testing.T) {
	assert.False(t, profileToProto(&user.Profile{ID: "p"}).HasPin)
	assert.False(t, profileToProto(&user.Profile{ID: "p", Parental: &user.ParentalSettings{}}).HasPin)
	assert.True(t, profileToProto(&user.Profile{ID: "p", Parental: &user.ParentalSettings{PINHash: "h"}}).HasPin)
}
