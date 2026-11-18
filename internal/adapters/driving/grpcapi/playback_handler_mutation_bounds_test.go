package grpcapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

func seedResume(t *testing.T, repo interface {
	Upsert(context.Context, *playback.WatchProgress) error
}, profile string, ids ...string) {
	t.Helper()
	for i, id := range ids {
		require.NoError(t, repo.Upsert(context.Background(), &playback.WatchProgress{
			ID: "wp-" + id, ProfileID: shared.ProfileID(profile), MediaItemID: shared.MediaItemID(id),
			Position: mustPlaybackDurationMs(t, 30000), LastPlayedAt: time.Now().Add(-time.Duration(i) * time.Minute),
		}))
	}
}

// An age cap of 0 is the strictest there is, not "no cap": the resume rail
// of such a profile hides what it does not allow.
func TestPlaybackHandler_GetResumeItems_AnAgeCapOfZeroStillFilters(t *testing.T) {
	h, _, progressRepo, mediaQuery, profileRepo, _ := newTestPlaybackHandler()
	zero := 0
	require.NoError(t, profileRepo.Create(context.Background(), &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Little", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &zero, Enabled: true}}))
	seedResume(t, progressRepo, "profile-2", "mi-r")
	mediaQuery.Seed(&library.MediaItem{ID: "mi-r", Title: "Adult", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})

	resp, err := h.GetResumeItems(userCtx("user-2", "profile-2"), &eveyv1.GetResumeItemsRequest{Limit: 10})
	require.NoError(t, err)
	for _, p := range resp.Progress {
		assert.NotEqual(t, "mi-r", p.MediaItemId, "an R title reached a profile capped at age 0")
	}
}

// No limit asked means the default page (20 resume entries, 50 history
// rows), not an empty one: the database answers LIMIT 0 with nothing.
func TestPlaybackHandler_ListsWithoutALimitUseTheDefaultPage(t *testing.T) {
	h, _, progressRepo, mediaQuery, _, _ := newTestPlaybackHandler()
	ids := make([]string, 55)
	for i := range ids {
		ids[i] = fmt.Sprintf("mi-%02d", i)
		mediaQuery.Seed(&library.MediaItem{ID: shared.MediaItemID(ids[i]), Title: ids[i], Type: "movie"})
	}
	seedResume(t, progressRepo, "profile-1", ids...)

	resume, err := h.GetResumeItems(pbAdminCtx(), &eveyv1.GetResumeItemsRequest{})
	require.NoError(t, err)
	assert.Len(t, resume.Progress, 20)

	history, err := h.GetWatchHistory(pbAdminCtx(), &eveyv1.GetWatchHistoryRequest{})
	require.NoError(t, err)
	assert.Len(t, history.Items, 50)
}

// A page that comes back full may have more behind it: it carries a cursor.
// One that comes back short is the end.
func TestPlaybackHandler_GetWatchHistory_AFullPageCarriesACursor(t *testing.T) {
	h, _, progressRepo, _, _, _ := newTestPlaybackHandler()
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = fmt.Sprintf("mi-%d", i)
	}
	seedResume(t, progressRepo, "profile-1", ids...)

	full, err := h.GetWatchHistory(pbAdminCtx(), &eveyv1.GetWatchHistoryRequest{Limit: 4})
	require.NoError(t, err)
	require.Len(t, full.Items, 4)
	assert.NotEmpty(t, full.Pagination.NextCursor, "a page exactly full lost its next cursor")

	short, err := h.GetWatchHistory(pbAdminCtx(), &eveyv1.GetWatchHistoryRequest{Limit: 5})
	require.NoError(t, err)
	require.Len(t, short.Items, 4)
	assert.Empty(t, short.Pagination.NextCursor)
}
