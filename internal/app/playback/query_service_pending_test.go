package playback_test

import (
	"context"
	"testing"
	"time"

	appplayback "github.com/eveyproject/evey/internal/app/playback"
	"github.com/eveyproject/evey/internal/domain/playback"
	pbfakes "github.com/eveyproject/evey/internal/domain/playback/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The player's last ticks wait in the buffer for up to a flush interval. A
// media page read right after leaving the player must see them, or it offers
// Play instead of Resume (or resumes seconds too early).
func TestQueryService_GetWatchProgress_ReadsPendingTicks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := pbfakes.NewFakeWatchProgressRepository()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour) // never flushes on its own
	q := appplayback.NewQueryService(repo).WithPendingProgress(buf)

	// First viewing: nothing in the database yet, the position only in the buffer.
	t0 := time.Now()
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(42_000), LastPlayedAt: t0})
	got, err := q.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(42_000), got.Position.Milliseconds())

	// Flushed, then a fresher tick buffered: the fresher position wins.
	require.Equal(t, 1, buf.Flush(ctx))
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(47_000), LastPlayedAt: t0.Add(5 * time.Second)})
	got, err = q.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(47_000), got.Position.Milliseconds())

	// Another title, nothing anywhere: still not found.
	_, err = q.GetWatchProgress(ctx, "p1", "other")
	require.Error(t, err)
}

// A buffered tick older than the stored row (a late write) does not win.
func TestQueryService_GetWatchProgress_StaleBufferedTickLoses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := pbfakes.NewFakeWatchProgressRepository()
	now := time.Now()
	require.NoError(t, repo.Upsert(ctx, &playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(90_000), LastPlayedAt: now}))
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(10_000), LastPlayedAt: now.Add(-time.Minute)})
	got, err := appplayback.NewQueryService(repo).WithPendingProgress(buf).GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(90_000), got.Position.Milliseconds())
}
