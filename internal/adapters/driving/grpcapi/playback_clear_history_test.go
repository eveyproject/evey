package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/user"
)

// ClearWatchHistory wipes ONE profile's rows and reports the count; a viewer
// cannot aim it at someone else's profile.
func TestClearWatchHistory(t *testing.T) {
	h, _, progressRepo, _, profileRepo, _ := newTestPlaybackHandler()
	ctx := context.Background()
	require.NoError(t, profileRepo.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Other"}))
	for _, wp := range []*playback.WatchProgress{
		{ID: "w1", ProfileID: "profile-1", MediaItemID: "m1"},
		{ID: "w2", ProfileID: "profile-1", MediaItemID: "m2"},
		{ID: "w3", ProfileID: "profile-2", MediaItemID: "m1"},
	} {
		require.NoError(t, progressRepo.Upsert(ctx, wp))
	}

	resp, err := h.ClearWatchHistory(pbAdminCtx(), &eveyv1.ClearWatchHistoryRequest{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, resp.Deleted, "only the caller's profile is cleared")
	if _, getErr := progressRepo.GetByProfileAndMedia(ctx, "profile-2", "m1"); getErr != nil {
		t.Errorf("the other profile's history must survive: %v", getErr)
	}

	// A viewer targeting another profile is refused; nothing is deleted.
	_, err = h.ClearWatchHistory(userCtx("user-1", "profile-1"), &eveyv1.ClearWatchHistoryRequest{ProfileId: "profile-2"})
	require.Error(t, err)
	if _, getErr := progressRepo.GetByProfileAndMedia(ctx, "profile-2", "m1"); getErr != nil {
		t.Errorf("a refused call must not delete: %v", getErr)
	}

	// Admins included: the call is pinned to the caller's own profile.
	_, err = h.ClearWatchHistory(pbAdminCtx(), &eveyv1.ClearWatchHistoryRequest{ProfileId: "profile-2"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// Naming one's own profile explicitly is fine; a second clear finds nothing.
	resp, err = h.ClearWatchHistory(pbAdminCtx(), &eveyv1.ClearWatchHistoryRequest{ProfileId: "profile-1"})
	require.NoError(t, err)
	assert.EqualValues(t, 0, resp.Deleted)

	_, err = h.ClearWatchHistory(context.Background(), &eveyv1.ClearWatchHistoryRequest{})
	require.Error(t, err)
}
