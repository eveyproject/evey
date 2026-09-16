package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The QoE feedback caps: one row per source class, rewritten in place, an
// expired cap is invisible to the handler and swept by DeleteExpired, a
// manual delete reports whether there was anything to delete.
func TestTranscodeCapsOverrideRepo_UpsertListDeleteExpire(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewTranscodeCapsOverrideRepo(db)
	now := time.Now().UTC().Truncate(time.Millisecond)

	live := transcoding.CapOverride{SourceClass: "hevc/2160p", MaxHeight: 1080, Reason: "starved", StarvedRatio: 0.42, SamplesCount: 120, AppliedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	stale := transcoding.CapOverride{SourceClass: "h264/1080p", MaxHeight: 720, Reason: "starved", StarvedRatio: 0.3, SamplesCount: 50, AppliedAt: now.Add(-48 * time.Hour), ExpiresAt: now.Add(-time.Hour)}
	require.NoError(t, repo.Upsert(ctx, live))
	require.NoError(t, repo.Upsert(ctx, stale))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1, "an expired cap is not served")
	assert.Equal(t, "hevc/2160p", list[0].SourceClass)
	assert.Equal(t, 1080, list[0].MaxHeight)
	assert.InDelta(t, 0.42, list[0].StarvedRatio, 1e-9)
	assert.Equal(t, 120, list[0].SamplesCount)

	live.MaxHeight = 720
	live.SamplesCount = 300
	require.NoError(t, repo.Upsert(ctx, live))
	list, err = repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1, "same class = same row")
	assert.Equal(t, 720, list[0].MaxHeight)
	assert.Equal(t, 300, list[0].SamplesCount)

	n, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the stale row is swept")

	deleted, err := repo.Delete(ctx, "hevc/2160p")
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.Delete(ctx, "hevc/2160p")
	require.NoError(t, err)
	assert.False(t, deleted, "nothing left to delete")
	list, err = repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)

	// No QoE rows at all: the aggregation is empty, not an error.
	rows, err := repo.AggregateStarvation(ctx, time.Hour, 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
}
