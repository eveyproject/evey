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

// A short intro is left alone: the prior only shrinks, it never extends.
func TestReconcileSeasonIntroDurations_NeverExtendsShortIntro(t *testing.T) {
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
			dur = 8_000 // undershoot - a real failure mode, but extension risks worse
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
