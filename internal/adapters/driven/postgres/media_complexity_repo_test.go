package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedComplexityItem inserts one media item with a file of the given type and
// duration. Type and duration are what ListUnclassified must discriminate on.
func seedComplexityItem(t *testing.T, itemRepo *postgres.MediaItemRepository, libID shared.LibraryID, id string, typ library.MediaItemType, duration time.Duration) {
	t.Helper()
	item := newTestMediaItem(id, libID)
	item.Type = typ
	d, err := shared.NewDurationFromTime(duration)
	require.NoError(t, err)
	item.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Duration: d}
	require.NoError(t, itemRepo.Create(context.Background(), item))
}

func TestComplexityRepo_ListUnclassified_OnlyClassifiableItems(t *testing.T) {
	// The classifier samples VIDEO frames. Tracks and photos can never classify,
	// and a zero-duration file gives the sampler nothing to spread over: the
	// worker skips all three WITHOUT persisting, so serving them as candidates
	// re-queues the same IDs forever (observed in prod 2026-07-26: ~24 silent
	// attempts/min around the clock on a library with a music section).
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaComplexityRepo(db)
	ctx := context.Background()

	lib := newTestLibrary("cx-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	seedComplexityItem(t, itemRepo, lib.ID, "cx-movie", library.MovieItem, 90*time.Minute)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-episode", library.Episode, 40*time.Minute)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-track", library.Track, 3*time.Minute)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-photo", library.PhotoItem, 0)
	seedComplexityItem(t, itemRepo, lib.ID, "cx-noduration", library.MovieItem, 0)

	ids, err := repo.ListUnclassified(ctx, 50)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"cx-movie", "cx-episode"}, ids,
		"only video items with a known duration are classifiable candidates")
}

// Get/Set round-trip, and the probe queue: a title enters it when its
// classifier result trips a trigger (no motion sampled, animation, motion
// outside the fitted range) and leaves it only once a probe stamped with the
// CURRENT identity (protocol + model version + encoder) is persisted - a
// failed probe included, so nothing is retried until an identity bump.
func TestComplexityRepo_SetGet_AndProbeCandidates(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("cx-lib2")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaComplexityRepo(db)
	for _, id := range []string{"cx-fit", "cx-fast", "cx-anim", "cx-nomotion", "cx-probed", "cx-stale-probe", "cx-legacy"} {
		seedComplexityItem(t, itemRepo, lib.ID, id, library.MovieItem, 90*time.Minute)
	}
	model := domaintranscoding.CostModelV2
	set := func(id string, class domaintranscoding.ContentClass, m domaintranscoding.ComplexityMetrics) {
		t.Helper()
		require.NoError(t, repo.Set(ctx, id, class, m, time.Now()))
	}
	fit := domaintranscoding.ComplexityMetrics{MotionMean: 3, FramesSampled: 500, MetricVersion: 2}
	set("cx-fit", domaintranscoding.ClassFilmNormal, fit)
	set("cx-fast", domaintranscoding.ClassFilmNormal, domaintranscoding.ComplexityMetrics{MotionMean: model.YDIFMax + 1, FramesSampled: 500, MetricVersion: 2})
	set("cx-anim", domaintranscoding.ClassAnimation, fit)
	set("cx-nomotion", domaintranscoding.ClassFilmNormal, domaintranscoding.ComplexityMetrics{MotionMean: 0, FramesSampled: model.MinFrames - 1, MetricVersion: 2})
	current := &domaintranscoding.TitleProbe{Protocol: domaintranscoding.TitleProbeProtocol, ModelVersion: model.Version, Encoder: domaintranscoding.TitleProbeEncoderID, Height: 1080}
	set("cx-probed", domaintranscoding.ClassAnimation, domaintranscoding.ComplexityMetrics{MotionMean: 3, FramesSampled: 500, MetricVersion: 2, Probe: current})
	stale := *current
	stale.ModelVersion = model.Version - 1
	set("cx-stale-probe", domaintranscoding.ClassAnimation, domaintranscoding.ComplexityMetrics{MotionMean: 3, FramesSampled: 500, MetricVersion: 2, Probe: &stale})
	// A pre-canonical row (metric_version 1) is the cheap classifier's job first.
	set("cx-legacy", domaintranscoding.ClassAnimation, domaintranscoding.ComplexityMetrics{MotionMean: 3, FramesSampled: 500, MetricVersion: 1})

	got, ok, err := repo.Get(ctx, "cx-probed")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, domaintranscoding.ClassAnimation, got.Class)
	require.NotNil(t, got.Metrics.Probe)
	assert.Equal(t, domaintranscoding.TitleProbeEncoderID, got.Metrics.Probe.Encoder)
	assert.False(t, got.ComputedAt.IsZero())
	_, ok, err = repo.Get(ctx, "ghost")
	require.NoError(t, err)
	assert.False(t, ok, "an unknown item is a miss, not an error")
	require.Error(t, repo.Set(ctx, "cx-fit", "", fit, time.Now()), "an empty class would leave both queues for good")
	require.Error(t, repo.Set(ctx, "ghost", domaintranscoding.ClassFilmNormal, fit, time.Now()))

	ids, err := repo.ListProbeCandidates(ctx, model.Version, 50)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"cx-fast", "cx-anim", "cx-nomotion", "cx-stale-probe"}, ids,
		"in-range film is not probed; a current probe settles; a stale probe or a legacy row does not qualify the same way")

	unclassified, err := repo.ListUnclassified(ctx, 50)
	require.NoError(t, err)
	assert.Empty(t, unclassified, "every seeded title carries a class")
}
