package playback_test

import (
	"context"
	"testing"
	"time"

	appplayback "github.com/eveyproject/evey/internal/app/playback"
	"github.com/eveyproject/evey/internal/domain/playback"
	pbfakes "github.com/eveyproject/evey/internal/domain/playback/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryService_NilSessions_ReturnsNil(t *testing.T) {
	t.Parallel()
	pr := pbfakes.NewFakeWatchProgressRepository()
	q := appplayback.NewQueryService(pr)

	sessions, err := q.GetActiveSessions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestQueryService_GetNextUp(t *testing.T) {
	t.Parallel()
	pr := pbfakes.NewFakeWatchProgressRepository()
	q := appplayback.NewQueryService(pr)
	ctx := context.Background()

	items, err := q.GetNextUp(ctx, "p1", 10)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestQueryService_GetSeriesPlayTarget(t *testing.T) {
	t.Parallel()
	pr := pbfakes.NewFakeWatchProgressRepository()
	pr.SeriesEpisodes = map[string][]playback.SeriesEpisode{
		"show": {
			{MediaItemID: "s1e1", Completed: true, LastPlayedAt: time.Unix(10, 0)},
			{MediaItemID: "s2e1"},
		},
	}
	q := appplayback.NewQueryService(pr)
	ctx := context.Background()

	got, started, ok, err := q.GetSeriesPlayTarget(ctx, "p1", "show")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, started)
	assert.Equal(t, shared.MediaItemID("s2e1"), got.MediaItemID)

	_, started, ok, err = q.GetSeriesPlayTarget(ctx, "p1", "empty")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.False(t, started)
}
