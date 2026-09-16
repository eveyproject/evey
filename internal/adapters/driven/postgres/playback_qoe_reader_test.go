package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
)

// The admin QoE dashboard: headline counters over a window, the titles
// viewers struggled with (volume-guarded so a 1-event film at 100% cannot
// top the list, labelled from the library), and the split per device class
// with blanks folded into "unknown".
func TestPlaybackQoEReader_SummaryTopStarvedAndByDevice(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewPlaybackQoERepo(db)
	reader := postgres.NewPlaybackQoEReader(db)

	lib := newTestLibrary("qoelib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	starvedID := uuid.NewString()
	item := newTestMediaItem(starvedID, lib.ID)
	item.Title = "Struggling Title"
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	quietID := uuid.NewString()
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, newTestMediaItem(quietID, lib.ID)))
	rareID := uuid.NewString()

	insert := func(mediaID, session, device string, starved bool, bufMs, kbps int) {
		t.Helper()
		require.NoError(t, repo.Insert(ctx, postgres.QoEEventRow{
			MediaItemID: &mediaID, SessionID: session, ContentID: mediaID, ObjectType: "v", StreamingFormat: "h", StreamType: "v",
			BufferStarved: starved, BufferLengthMs: &bufMs, EncodedKbps: &kbps, DeviceClass: device, Raw: map[string]string{},
		}))
	}
	// 60 events on the struggling title, 20 of them starved, two sessions.
	for i := range 60 {
		insert(starvedID, "s-"+string(rune('a'+i%2)), "tv", i%3 == 0, 2000, 6000)
	}
	// 60 quiet events on another title: never starved, blank device class.
	for range 60 {
		insert(quietID, "s-q", "", false, 8000, 4000)
	}
	// A rare title starved on its only 3 events: below the volume guard.
	for range 3 {
		insert(rareID, "s-r", "web", true, 100, 1000)
	}

	sum, err := reader.GetSummary(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "24h", sum.WindowLabel)
	assert.EqualValues(t, 123, sum.TotalEvents)
	assert.EqualValues(t, 23, sum.StarvedEvents)
	assert.InDelta(t, 23.0/123.0, sum.StarvedRatio, 1e-9)
	assert.EqualValues(t, 4, sum.UniqueSessions)
	assert.EqualValues(t, 3, sum.UniqueMedia)

	top, err := reader.GetTopStarvedTitles(ctx, 24*time.Hour, 10)
	require.NoError(t, err)
	require.Len(t, top, 1, "the quiet title has no starved event, the rare one is under the volume guard")
	assert.Equal(t, starvedID, top[0].MediaItemID)
	assert.Equal(t, "Struggling Title", top[0].Title)
	assert.EqualValues(t, 20, top[0].StarvedEvents)
	assert.EqualValues(t, 60, top[0].TotalEvents)
	assert.False(t, top[0].LastStarvedAt.IsZero())

	byDevice, err := reader.GetByDeviceClass(ctx, 24*time.Hour)
	require.NoError(t, err)
	classes := map[string]postgres.QoEByDevice{}
	for _, d := range byDevice {
		classes[d.DeviceClass] = d
	}
	assert.EqualValues(t, 60, classes["tv"].TotalEvents)
	assert.EqualValues(t, 20, classes["tv"].StarvedEvents)
	assert.InDelta(t, 2000, classes["tv"].AvgBufferMs, 1e-6)
	assert.EqualValues(t, 60, classes["unknown"].TotalEvents, "a blank class folds into unknown so totals add up")
	assert.Zero(t, classes["unknown"].StarvedEvents)
	assert.EqualValues(t, 3, classes["web"].TotalEvents)

	// Outside the window everything is empty, not an error.
	_, err = db.ExecContext(ctx, `UPDATE playback_qoe_events SET ts = now() - interval '3 days'`)
	require.NoError(t, err)
	sum, err = reader.GetSummary(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Zero(t, sum.TotalEvents)
	assert.Zero(t, sum.StarvedRatio)
	sum, err = reader.GetSummary(ctx, 7*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "7d", sum.WindowLabel)
	assert.EqualValues(t, 123, sum.TotalEvents)
}
