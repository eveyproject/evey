package grpcapi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// wireActivityAccess gives the handler the three ports the history gate reads
// (library items, profiles, library grants), as server.go does.
func wireActivityAccess(h *activityHandler, mediaRepo *libfakes.FakeMediaItemQueryRepo, profileRepo *userfakes.FakeProfileRepository) {
	userRepo := userfakes.NewFakeUserRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	h.libQuery = applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	h.userAccess = appuser.NewAccessService(userRepo, apiKeyRepo, inviteRepo, qcRepo,
		userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(userfakes.NewFakeLibraryAccessRepository()))
	// A caller whose profile cannot be loaded is treated as most restricted, so
	// the ungated tests need a real, plain profile.
	_ = profileRepo.Create(context.Background(), &user.Profile{ID: "profile-1", UserID: "user-1", Name: "Plain"})
	h.userQuery = appuser.NewQueryService(userRepo, profileRepo, apiKeyRepo, inviteRepo, qcRepo)
}

func TestActivityHandler_GetPlayHistory_ParentalFiltersAdult(t *testing.T) {
	h, _, _, playHistory, _ := newActivityHandler()
	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	profileRepo := userfakes.NewFakeProfileRepository()
	wireActivityAccess(h, mediaRepo, profileRepo)
	ctx := context.Background()

	maxAge := 13
	profileRepo.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-r", Title: "Adult Movie", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-pg", Title: "Some Movie", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}})

	now := time.Now()
	playHistory.Add(
		&activity.PlayHistoryEntry{ID: "ph-r", ProfileID: "profile-2", MediaItemID: "mi-r", MediaTitle: "Adult Movie", MediaType: "movie", StartedAt: now},
		&activity.PlayHistoryEntry{ID: "ph-pg", ProfileID: "profile-2", MediaItemID: "mi-pg", MediaTitle: "Some Movie", MediaType: "movie", StartedAt: now.Add(-time.Minute)},
		&activity.PlayHistoryEntry{ID: "ph-gone", ProfileID: "profile-2", MediaItemID: "mi-gone", MediaTitle: "Removed Movie", MediaType: "movie", StartedAt: now.Add(-2 * time.Minute)},
	)

	resp, err := h.GetPlayHistory(userCtx("user-2", "profile-2"), &eveyv1.GetPlayHistoryRequest{})
	require.NoError(t, err)
	ids := make([]string, len(resp.Entries))
	for i, e := range resp.Entries {
		ids[i] = e.MediaItemId
	}
	assert.NotContains(t, ids, "mi-r", "adult history entry must be hidden from a kids profile")
	assert.NotContains(t, ids, "mi-gone", "an unresolvable item stays hidden for a gated profile")
	assert.Contains(t, ids, "mi-pg", "kid-safe history entry must remain")
}

func TestActivityHandler_GetPlayHistory_UngatedKeepsDeletedMedia(t *testing.T) {
	h, _, _, playHistory, _ := newActivityHandler()
	wireActivityAccess(h, libfakes.NewFakeMediaItemQueryRepo(), userfakes.NewFakeProfileRepository())
	playHistory.Add(&activity.PlayHistoryEntry{ID: "ph-gone", ProfileID: "profile-1", MediaItemID: "mi-gone",
		MediaTitle: "Removed Movie", MediaType: "movie", StartedAt: time.Now()})

	resp, err := h.GetPlayHistory(ctxWithClaims("user-1", "profile-1", false), &eveyv1.GetPlayHistoryRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Entries, 1)
}
