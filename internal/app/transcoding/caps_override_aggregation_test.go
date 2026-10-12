package transcoding

// Integration tests for aggregateAndUpsert - the SQL feedback loop that
// writes cap overrides when a source class shows sustained starvation.
//
// These tests require a real postgres instance spun up via testcontainers.
// They are skipped with -short so the unit suite stays fast.

import (
	"context"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgadapter "github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
)

// TestAggregateAndUpsert_EmptyAggregation_NoOverrides confirms that when the
// repo's aggregation returns no buckets (the fake returns nil), the worker
// writes no overrides and doesn't error - the unit-level seam reachable without
// a real database (the SQL itself is integration-tested below).
func TestAggregateAndUpsert_EmptyAggregation_NoOverrides(t *testing.T) {
	repo := &fakeCapOverrideRepo{}
	svc := NewCapOverrideService(repo, CapOverrideServiceOpts{}, nil)
	err := svc.aggregateAndUpsert(context.Background())
	require.NoError(t, err, "empty aggregation must not error")
	assert.Empty(t, repo.rows, "empty aggregation must not produce any override rows")
}

// TestAggregateAndUpsert_StarvedBatch_WritesOverride seeds a 4K HEVC source
// with a batch of QoE events where >30 % are buffer-starved, then calls
// aggregateAndUpsert and asserts:
//   - a transcode_caps_overrides row is written for "hevc/2160p"
//   - the cap is lowered to 1080 (one rung below 4K)
//   - starved_ratio is correct
func TestAggregateAndUpsert_StarvedBatch_WritesOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test under -short")
	}

	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()

	// Seed the minimal FK chain:  library → media_item → media_stream.
	const libID = "lib-caps-test"
	_, err := db.ExecContext(ctx, `
		INSERT INTO libraries (id, name, type, paths)
		VALUES ($1, 'Test Library', 'movie', '[]')`, libID)
	require.NoError(t, err, "insert library")

	const mediaID = "mi-caps-4k-hevc"
	_, err = db.ExecContext(ctx, `
		INSERT INTO media_items (id, library_id, type, title, sort_title)
		VALUES ($1, $2, 'movie', '4K HEVC Film', '4k hevc film')`, mediaID, libID)
	require.NoError(t, err, "insert media_item")

	// A video stream: 4K, HEVC codec - matches the "hevc/2160p" bucket.
	_, err = db.ExecContext(ctx, `
		INSERT INTO media_streams (id, media_item_id, stream_index, stream_type, codec, height)
		VALUES ('ms-caps-vid-1', $1, 0, 'video', 'hevc', 2160)`, mediaID)
	require.NoError(t, err, "insert media_stream")

	// Seed 60 QoE events: 30 starved (50 % > 30 % threshold), 30 clean.
	// Use ts = NOW() so they fall within the default 24h AggregationWindow.
	for i := range 60 {
		starved := i < 30
		_, err = db.ExecContext(ctx, `
			INSERT INTO playback_qoe_events (media_item_id, buffer_starved, measured_kbps, ts)
			VALUES ($1, $2, $3, NOW())`, mediaID, starved, 500)
		require.NoError(t, err, "insert qoe event %d", i)
	}

	// Wire up the real postgres repo so we can read back what was upserted.
	repo := pgadapter.NewTranscodeCapsOverrideRepo(db)
	svc := NewCapOverrideService(repo, CapOverrideServiceOpts{
		AggregationWindow: 24 * time.Hour,
		MinSamples:        50, // 60 total ≥ 50
		StarvedThreshold:  0.30,
		OverrideTTL:       7 * 24 * time.Hour,
	}, nil)

	err = svc.aggregateAndUpsert(ctx)
	require.NoError(t, err)

	// Read back the written overrides.
	overrides, err := repo.List(ctx)
	require.NoError(t, err)

	require.Len(t, overrides, 1, "exactly one override row expected for hevc/2160p")
	o := overrides[0]
	assert.Equal(t, "hevc/2160p", o.SourceClass)
	assert.Equal(t, 1080, o.MaxHeight, "4K HEVC bucket must be capped one rung lower at 1080p")
	assert.InDelta(t, 0.50, o.StarvedRatio, 0.01, "starved_ratio must reflect the seeded 30/60 ratio")
	assert.Equal(t, 60, o.SamplesCount)
	assert.True(t, o.ExpiresAt.After(time.Now()), "override must not already be expired")
}

// TestAggregateAndUpsert_NonStarvedBatch_NoOverride seeds a 4K H264 source
// with a batch below the starvation threshold (10 %) and confirms no override
// row is written - the service must not write caps for healthy source classes.
func TestAggregateAndUpsert_NonStarvedBatch_NoOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test under -short")
	}

	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()

	const libID = "lib-caps-healthy"
	_, err := db.ExecContext(ctx, `
		INSERT INTO libraries (id, name, type, paths)
		VALUES ($1, 'Healthy Library', 'movie', '[]')`, libID)
	require.NoError(t, err)

	const mediaID = "mi-caps-4k-h264"
	_, err = db.ExecContext(ctx, `
		INSERT INTO media_items (id, library_id, type, title, sort_title)
		VALUES ($1, $2, 'movie', '4K H264 Film', '4k h264 film')`, mediaID, libID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO media_streams (id, media_item_id, stream_index, stream_type, codec, height)
		VALUES ('ms-healthy-vid-1', $1, 0, 'video', 'h264', 2160)`, mediaID)
	require.NoError(t, err)

	// 60 events, only 6 starved (10 % < 30 % threshold).
	for i := range 60 {
		starved := i < 6
		_, err = db.ExecContext(ctx, `
			INSERT INTO playback_qoe_events (media_item_id, buffer_starved, measured_kbps, ts)
			VALUES ($1, $2, $3, NOW())`, mediaID, starved, 4000)
		require.NoError(t, err)
	}

	repo := pgadapter.NewTranscodeCapsOverrideRepo(db)
	svc := NewCapOverrideService(repo, CapOverrideServiceOpts{
		AggregationWindow: 24 * time.Hour,
		MinSamples:        50,
		StarvedThreshold:  0.30,
		OverrideTTL:       7 * 24 * time.Hour,
	}, nil)

	err = svc.aggregateAndUpsert(ctx)
	require.NoError(t, err)

	overrides, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, overrides, "healthy source class must not produce an override")
}

// TestAggregateAndUpsert_CodecBuckets_IsolatedPerCodec verifies that a
// starved HEVC batch does NOT produce an override for the H264 bucket
// (per-codec granularity must not bleed across codecs).
func TestAggregateAndUpsert_CodecBuckets_IsolatedPerCodec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test under -short")
	}

	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()

	const libID = "lib-caps-codec-iso"
	_, err := db.ExecContext(ctx, `
		INSERT INTO libraries (id, name, type, paths)
		VALUES ($1, 'Codec Isolation Library', 'movie', '[]')`, libID)
	require.NoError(t, err)

	// Two separate media items: one HEVC 4K (starved), one H264 4K (healthy).
	type item struct {
		mediaID  string
		streamID string
		codec    string
		height   int
	}
	items := []item{
		{"mi-hevc-4k", "ms-hevc-4k-v", "hevc", 2160},
		{"mi-h264-4k", "ms-h264-4k-v", "h264", 2160},
	}

	for _, it := range items {
		_, err = db.ExecContext(ctx, `
			INSERT INTO media_items (id, library_id, type, title, sort_title)
			VALUES ($1, $2, 'movie', $1, $1)`, it.mediaID, libID)
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `
			INSERT INTO media_streams (id, media_item_id, stream_index, stream_type, codec, height)
			VALUES ($1, $2, 0, 'video', $3, $4)`, it.streamID, it.mediaID, it.codec, it.height)
		require.NoError(t, err)
	}

	// HEVC: 60 events, 50 % starved → triggers override.
	for i := range 60 {
		_, err = db.ExecContext(ctx, `
			INSERT INTO playback_qoe_events (media_item_id, buffer_starved, ts)
			VALUES ($1, $2, NOW())`, "mi-hevc-4k", i < 30)
		require.NoError(t, err)
	}
	// H264: 60 events, 0 % starved → no override.
	for range 60 {
		_, err = db.ExecContext(ctx, `
			INSERT INTO playback_qoe_events (media_item_id, buffer_starved, ts)
			VALUES ($1, false, NOW())`, "mi-h264-4k")
		require.NoError(t, err)
	}

	repo := pgadapter.NewTranscodeCapsOverrideRepo(db)
	svc := NewCapOverrideService(repo, CapOverrideServiceOpts{
		AggregationWindow: 24 * time.Hour,
		MinSamples:        50,
		StarvedThreshold:  0.30,
		OverrideTTL:       7 * 24 * time.Hour,
	}, nil)

	err = svc.aggregateAndUpsert(ctx)
	require.NoError(t, err)

	overrides, err := repo.List(ctx)
	require.NoError(t, err)

	require.Len(t, overrides, 1, "only one override expected - for hevc/2160p, not h264/2160p")
	assert.Equal(t, "hevc/2160p", overrides[0].SourceClass,
		"per-codec starvation must not bleed into other codec bucket")
}

// TestAggregateAndUpsert_BelowMinSamples_NoOverride verifies that a bucket
// with very high starvation but fewer than MinSamples events is NOT acted
// upon - too noisy to be reliable.
func TestAggregateAndUpsert_BelowMinSamples_NoOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres integration test under -short")
	}

	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()

	const libID = "lib-caps-noisy"
	_, err := db.ExecContext(ctx, `
		INSERT INTO libraries (id, name, type, paths)
		VALUES ($1, 'Noisy Library', 'movie', '[]')`, libID)
	require.NoError(t, err)

	const mediaID = "mi-noisy-hevc"
	_, err = db.ExecContext(ctx, `
		INSERT INTO media_items (id, library_id, type, title, sort_title)
		VALUES ($1, $2, 'movie', 'Noisy HEVC', 'noisy hevc')`, mediaID, libID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO media_streams (id, media_item_id, stream_index, stream_type, codec, height)
		VALUES ('ms-noisy-vid', $1, 0, 'video', 'hevc', 2160)`, mediaID)
	require.NoError(t, err)

	// Only 10 events, all starved (100 % > threshold) - but 10 < MinSamples=50.
	for range 10 {
		_, err = db.ExecContext(ctx, `
			INSERT INTO playback_qoe_events (media_item_id, buffer_starved, ts)
			VALUES ($1, true, NOW())`, mediaID)
		require.NoError(t, err)
	}

	repo := pgadapter.NewTranscodeCapsOverrideRepo(db)
	svc := NewCapOverrideService(repo, CapOverrideServiceOpts{
		AggregationWindow: 24 * time.Hour,
		MinSamples:        50, // 10 events < 50 → filtered out by SQL WHERE total >= $2
		StarvedThreshold:  0.30,
		OverrideTTL:       7 * 24 * time.Hour,
	}, nil)

	err = svc.aggregateAndUpsert(ctx)
	require.NoError(t, err)

	overrides, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, overrides, "bucket below MinSamples must not generate an override despite high starved ratio")
}
