package grpcapi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/user"
)

func TestPlaybackHandler_GetWatchHistory_ParentalFiltersAdult(t *testing.T) {
	h, _, progressRepo, mediaQuery, profileRepo, _ := newTestPlaybackHandler()
	ctx := context.Background()

	maxAge := 13
	profileRepo.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})

	progressRepo.Upsert(ctx, &playback.WatchProgress{ID: "wp-r", ProfileID: "profile-2", MediaItemID: "mi-r",
		Position: mustPlaybackDurationMs(t, 30000), Completed: true, LastPlayedAt: time.Now()})
	progressRepo.Upsert(ctx, &playback.WatchProgress{ID: "wp-pg", ProfileID: "profile-2", MediaItemID: "mi-pg",
		Position: mustPlaybackDurationMs(t, 30000), Completed: true, LastPlayedAt: time.Now()})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-r", Title: "Adult Movie", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-pg", Title: "Some Movie", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}})

	resp, err := h.GetWatchHistory(userCtx("user-2", "profile-2"), &eveyv1.GetWatchHistoryRequest{Limit: 10})
	require.NoError(t, err)
	ids := make([]string, len(resp.Items))
	for i, it := range resp.Items {
		ids[i] = it.MediaItemId
	}
	assert.NotContains(t, ids, "mi-r", "adult history item must be hidden from a kids profile")
	assert.Contains(t, ids, "mi-pg", "kid-safe history item must remain")
}
