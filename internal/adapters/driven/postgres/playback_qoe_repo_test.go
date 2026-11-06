package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The retention sweep for this table is wired and unit-tested against a FAKE
// purger, which proves the scheduling and nothing about the SQL. This table is
// the busiest write path in the server (one row per CMCD-bearing segment), so
// the statement that bounds it deserves to be exercised against a real
// database: a purge that deleted nothing, or everything, would look identical
// from the app layer.
func TestPlaybackQoERepo_PurgeRemovesOnlyRowsPastTheWindow(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaybackQoERepo(db)
	ctx := context.Background()

	// ts defaults to NOW(), so both rows land "fresh" and one is backdated
	// afterwards - the only way to age a row without waiting 30 days.
	for _, sid := range []string{"sess-old", "sess-recent"} {
		require.NoError(t, repo.Insert(ctx, postgres.QoEEventRow{
			SessionID:       sid,
			ContentID:       "media-1",
			ObjectType:      "v",
			StreamingFormat: "h",
			StreamType:      "v",
			Raw:             map[string]string{"sid": sid},
		}))
	}
	_, err := db.ExecContext(ctx,
		`UPDATE playback_qoe_events SET ts = now() - interval '60 days' WHERE session_id = $1`, "sess-old")
	require.NoError(t, err)

	removed, err := repo.Purge(ctx, time.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed, "only the row past the window may be deleted")

	// The surviving row is the point of the assertion: without it, a purge that
	// emptied the table would pass on the count alone.
	var remaining []string
	rows, err := db.QueryContext(ctx, `SELECT session_id FROM playback_qoe_events ORDER BY session_id`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		remaining = append(remaining, s)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"sess-recent"}, remaining, "the in-window row must survive")
}

// A window that predates every row must be a no-op: the daily sweep runs on
// every boot, and a purge that ignored its cutoff would wipe live telemetry.
func TestPlaybackQoERepo_PurgeKeepsEverythingInsideTheWindow(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaybackQoERepo(db)
	ctx := context.Background()
	require.NoError(t, repo.Insert(ctx, postgres.QoEEventRow{
		SessionID: "sess-1", ContentID: "media-1",
		ObjectType: "v", StreamingFormat: "h", StreamType: "v",
	}))

	removed, err := repo.Purge(ctx, time.Now().Add(-365*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), removed)

	var n int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM playback_qoe_events`).Scan(&n))
	assert.Equal(t, 1, n)
}

// A batch lands whole, every row with its own values: the sink hands the
// repository what piled up behind a slow write, and a tuple shifted by one
// column would store one viewer's figures under another's session.
func TestPlaybackQoERepo_InsertMany_LandsEveryRowWithItsOwnValues(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaybackQoERepo(db)
	ctx := context.Background()

	rows := make([]postgres.QoEEventRow, 0, 3)
	for i := range 3 {
		rows = append(rows, postgres.QoEEventRow{
			SessionID:      "sess-batch",
			ContentID:      "media-1",
			SegmentIdx:     new(i),
			BufferLengthMs: new(1000 * (i + 1)),
			ObjectType:     "v",
			Raw:            map[string]string{"sid": "sess-batch"},
			RemoteIP:       "",
		})
	}
	require.NoError(t, repo.InsertMany(ctx, rows))
	require.NoError(t, repo.InsertMany(ctx, nil), "an empty batch is a no-op")

	got, err := db.QueryContext(ctx,
		`SELECT segment_idx, buffer_length_ms FROM playback_qoe_events WHERE session_id = $1 ORDER BY segment_idx`, "sess-batch")
	require.NoError(t, err)
	defer got.Close()
	var seen []int
	for got.Next() {
		var seg, buf int
		require.NoError(t, got.Scan(&seg, &buf))
		assert.Equal(t, 1000*(seg+1), buf, "row %d keeps its own buffer length", seg)
		seen = append(seen, seg)
	}
	require.NoError(t, got.Err())
	assert.Equal(t, []int{0, 1, 2}, seen)
}
