package library

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

func TestSeasonIntroDurationStats(t *testing.T) {
	t.Parallel()
	t.Run("below min samples", func(t *testing.T) {
		t.Parallel()
		_, ok := seasonIntroDurationStats([]int64{30_000, 30_000, 30_000})
		assert.False(t, ok, "fewer than priorMinSamples durations must not form a prior")
	})
	t.Run("median and MAD", func(t *testing.T) {
		t.Parallel()
		st, ok := seasonIntroDurationStats([]int64{28_000, 30_000, 30_000, 31_000, 104_000})
		require.True(t, ok)
		assert.Equal(t, int64(30_000), st.medianMs)
		assert.Equal(t, int64(1_000), st.madMs, "MAD of {2000,0,0,1000,74000} is 1000")
		assert.Equal(t, 5, st.n)
	})
}

func TestDurationExceedsPrior(t *testing.T) {
	t.Parallel()
	st := introDurationStats{medianMs: 30_000, madMs: 1_000, n: 6}
	// 4*1.4826*1000 ≈ 5.9s < the 12s floor → the band is the floor.
	assert.False(t, durationExceedsPrior(30_000, st))
	assert.False(t, durationExceedsPrior(41_000, st), "inside the 12s floor band")
	assert.False(t, durationExceedsPrior(18_000, st), "short intros are never outliers - only the long side is policed")
	assert.True(t, durationExceedsPrior(43_000, st), "beyond the floor band")
	assert.True(t, durationExceedsPrior(104_000, st), "absorbed recap class")

	// A noisy season widens its own band: MAD 5s → 4*1.4826*5000 ≈ 29.7s.
	noisy := introDurationStats{medianMs: 30_000, madMs: 5_000, n: 6}
	assert.False(t, durationExceedsPrior(55_000, noisy))
	assert.True(t, durationExceedsPrior(62_000, noisy))
}

// A season where most episodes carry a ~30s intro and one detection absorbed
// the recap (104s) must shrink the outlier to [start, start+median], capping
// its confidence below direct detections.
func TestReconcileSeasonIntroDurations_ShrinksAbsorbedRecap(t *testing.T) {
	t.Parallel()
	segmentRepo := &fakeSegmentRepo{}
	var eps []*domainlibrary.MediaItem
	for i := range 5 {
		id := fmt.Sprintf("ep-%d", i)
		idx := i + 1
		eps = append(eps, &domainlibrary.MediaItem{
			ID: shared.MediaItemID(id), Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
		})
		dur := int64(30_000)
		conf := 0.9
		if i == 4 {
			dur = 104_000 // the absorbed-recap outlier
		}
		segmentRepo.seeded = append(segmentRepo.seeded, &domainlibrary.MediaSegment{
			MediaItemID: shared.MediaItemID(id), Type: "intro",
			StartMs: 10_000, EndMs: 10_000 + dur, Source: "audio_cluster", Confidence: conf,
		})
	}
	svc := NewIntroDetectionService(
		segmentRepo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	svc.reconcileSeasonIntroDurations(context.Background(), "season-id")

	require.Len(t, segmentRepo.upserted, 1, "only the outlier is rewritten")
	got := segmentRepo.upserted[0]
	assert.Equal(t, shared.MediaItemID("ep-4"), got.MediaItemID)
	assert.Equal(t, int64(10_000), got.StartMs, "the start is trusted - only the end shrinks")
	assert.Equal(t, int64(40_000), got.EndMs, "end = start + season median duration")
	assert.LessOrEqual(t, got.Confidence, 0.75)
	assert.Equal(t, "audio_cluster", got.Source, "the source survives - the prior corrects, it does not re-author")
}

// A manual intro is never rewritten, even when it is the duration outlier.
func TestReconcileSeasonIntroDurations_LeavesManualAlone(t *testing.T) {
	t.Parallel()
	segmentRepo := &fakeSegmentRepo{}
	var eps []*domainlibrary.MediaItem
	for i := range 5 {
		id := fmt.Sprintf("ep-%d", i)
		idx := i + 1
		eps = append(eps, &domainlibrary.MediaItem{
			ID: shared.MediaItemID(id), Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
		})
		src, dur := "audio_cluster", int64(30_000)
		if i == 4 {
			src, dur = "manual", int64(104_000)
		}
		segmentRepo.seeded = append(segmentRepo.seeded, &domainlibrary.MediaSegment{
			MediaItemID: shared.MediaItemID(id), Type: "intro",
			StartMs: 10_000, EndMs: 10_000 + dur, Source: src, Confidence: 1.0,
		})
	}
	svc := NewIntroDetectionService(
		segmentRepo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	svc.reconcileSeasonIntroDurations(context.Background(), "season-id")
	assert.Empty(t, segmentRepo.upserted, "a manual segment is excluded from the prior entirely")
}

// A CORROBORATED short intro (confidence at/above the support line) is left
// alone: the outlier rule never extends, and the gentle snap only touches
// lone-witness segments.
func TestReconcileSeasonIntroDurations_LeavesCorroboratedShortIntroAlone(t *testing.T) {
	t.Parallel()
	segmentRepo := &fakeSegmentRepo{}
	var eps []*domainlibrary.MediaItem
	for i := range 5 {
		id := fmt.Sprintf("ep-%d", i)
		idx := i + 1
		eps = append(eps, &domainlibrary.MediaItem{
			ID: shared.MediaItemID(id), Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
		})
		dur := int64(30_000)
		if i == 4 {
			dur = 8_000 // undershoot, but corroborated - respected as-is
		}
		segmentRepo.seeded = append(segmentRepo.seeded, &domainlibrary.MediaSegment{
			MediaItemID: shared.MediaItemID(id), Type: "intro",
			StartMs: 10_000, EndMs: 10_000 + dur, Source: "audio_cluster", Confidence: 0.9,
		})
	}
	svc := NewIntroDetectionService(
		segmentRepo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	svc.reconcileSeasonIntroDurations(context.Background(), "season-id")
	assert.Empty(t, segmentRepo.upserted)
}

// seedSeason builds a 5-episode season whose intro durations and confidences
// are given per episode, all sourced audio_cluster at start 10s.
func seedSeason(durMs []int64, conf []float64) (*fakeSegmentRepo, []*domainlibrary.MediaItem) {
	repo := &fakeSegmentRepo{}
	var eps []*domainlibrary.MediaItem
	for i := range durMs {
		id := fmt.Sprintf("ep-%d", i)
		idx := i + 1
		eps = append(eps, &domainlibrary.MediaItem{
			ID: shared.MediaItemID(id), Type: domainlibrary.Episode, IndexNumber: &idx,
			File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
		})
		repo.seeded = append(repo.seeded, &domainlibrary.MediaSegment{
			MediaItemID: shared.MediaItemID(id), Type: "intro",
			StartMs: 10_000, EndMs: 10_000 + durMs[i], Source: "audio_cluster", Confidence: conf[i],
		})
	}
	return repo, eps
}

// The music-bleed transfer: four episodes carry corroborated (subtitle-agreed)
// ends at ~30s, the fifth has a lone-audio end overshooting by 6s. Its
// duration snaps to the corroborated median - the subtitle signal of the
// siblings reaches the episode that has none.
func TestReconcileSeasonIntroDurations_SnapsLoneWitnessToCorroboratedMedian(t *testing.T) {
	t.Parallel()
	repo, eps := seedSeason(
		[]int64{30_000, 30_000, 29_500, 30_500, 36_000},
		[]float64{1.0, 1.0, 0.95, 0.9, 0.8},
	)
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	svc.reconcileSeasonIntroDurations(context.Background(), "season-id")

	require.Len(t, repo.upserted, 1)
	got := repo.upserted[0]
	assert.Equal(t, shared.MediaItemID("ep-4"), got.MediaItemID)
	assert.Equal(t, int64(10_000+30_000), got.EndMs, "lone-witness end adopts the corroborated median duration")
}

// The snap works in BOTH directions: a lone-witness end cut 6s short is
// extended to the corroborated median (the too-early-cutoff class).
func TestReconcileSeasonIntroDurations_SnapExtendsShortLoneWitness(t *testing.T) {
	t.Parallel()
	repo, eps := seedSeason(
		[]int64{30_000, 30_000, 29_500, 30_500, 24_000},
		[]float64{1.0, 1.0, 0.95, 0.9, 0.8},
	)
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	svc.reconcileSeasonIntroDurations(context.Background(), "season-id")

	require.Len(t, repo.upserted, 1)
	assert.Equal(t, int64(10_000+30_000), repo.upserted[0].EndMs)
}

// Inside the ±3s jitter floor nothing moves - correcting there would be noise
// chasing noise.
func TestReconcileSeasonIntroDurations_SnapRespectsJitterFloor(t *testing.T) {
	t.Parallel()
	repo, eps := seedSeason(
		[]int64{30_000, 30_000, 29_500, 30_500, 32_000},
		[]float64{1.0, 1.0, 0.95, 0.9, 0.8},
	)
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	svc.reconcileSeasonIntroDurations(context.Background(), "season-id")
	assert.Empty(t, repo.upserted)
}

// Without enough corroborated ends the snap has no trustworthy target and
// stays out entirely (only the outlier rule may act).
func TestReconcileSeasonIntroDurations_NoSnapWithoutCorroboratedSupport(t *testing.T) {
	t.Parallel()
	repo, eps := seedSeason(
		[]int64{30_000, 30_000, 29_500, 30_500, 36_000},
		[]float64{0.8, 0.8, 0.8, 0.8, 0.8},
	)
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	svc.reconcileSeasonIntroDurations(context.Background(), "season-id")
	assert.Empty(t, repo.upserted)
}
