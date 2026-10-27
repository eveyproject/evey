package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// The avatar path is the upload handler's to set (avatars/<profile id>.<ext>).
// A client-supplied one could point a profile at another profile's file, which
// the next upload then deletes as "the old avatar": it is never stored.
func TestProfileRPCs_NeverStoreAClientAvatarPath(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice"}, "pass12345678")
	seedAdultProfile(t, env, "prf-1", "usr-1")
	ctx := userCtx("usr-1", "prf-1")

	for _, path := range []string{"avatars/prf-other.png", "avatars/../../etc/passwd", "/tmp/x.png"} {
		resp, err := env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "New", AvatarPath: path})
		if err != nil {
			continue // refusing is fine too, storing is not
		}
		stored, gErr := env.profileRepo.GetByID(context.Background(), shared.ProfileID(resp.Id))
		require.NoError(t, gErr)
		assert.Nil(t, stored.AvatarPath, path)
		require.NoError(t, env.profileRepo.Delete(context.Background(), shared.ProfileID(resp.Id)))
	}

	other := "avatars/prf-other.png"
	_, _ = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", AvatarPath: &other})
	stored, err := env.profileRepo.GetByID(context.Background(), "prf-1")
	require.NoError(t, err)
	assert.Nil(t, stored.AvatarPath)
}
