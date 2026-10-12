package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// The leading-logo-block pass is a FILM-only signal: the ground-truth set
// labels the same distributor-logo-on-black pattern NEGATIVE on series
// ("Pas d'intro"). It must run for a movie and must NOT run for an episode.
func TestRunPipeline_LeadingLogoBlock_MovieOnly(t *testing.T) {
	t.Parallel()

	newDetector := func() *countingDetector {
		return &countingDetector{fakeDetector: fakeDetector{
			leadingLogoFound: true, leadingLogoStartMs: 0, leadingLogoEndMs: 35_000,
		}}
	}

	t.Run("movie: the pass runs and its result is stored", func(t *testing.T) {
		t.Parallel()
		det := newDetector()
		svc := NewIntroDetectionService(
			&fakeSegmentRepo{},
			&fakeTrickplayRepo{},
			&fakeMediaItemRepo{},
			&fakeConfigRepo{}, // disables trickplay: the pHash pass is skipped
			det,
			"/tmp/trickplay",
		)
		item := &domainlibrary.MediaItem{
			ID:   shared.MediaItemID("movie-1"),
			Type: domainlibrary.MovieItem,
			File: &domainlibrary.MediaFile{Path: "/tmp/movie.mkv"},
		}

		segs, err := svc.runPipeline(context.Background(), item, nil)
		require.NoError(t, err)

		require.Equal(t, int32(1), det.leadingLogoCalls.Load(), "the pass must run on a movie")
		require.Len(t, segs, 1)
		assert.Equal(t, "intro", segs[0].Type)
		assert.Equal(t, "leading_black", segs[0].Source)
		assert.Equal(t, int64(0), segs[0].StartMs)
		assert.Equal(t, int64(35_000), segs[0].EndMs)
		assert.InDelta(t, 0.7, segs[0].Confidence, 0.0001)
	})

	t.Run("episode: the pass never runs, even with the same detector answer", func(t *testing.T) {
		t.Parallel()
		det := newDetector()
		svc := NewIntroDetectionService(
			&fakeSegmentRepo{},
			&fakeTrickplayRepo{},
			&fakeMediaItemRepo{},
			&fakeConfigRepo{},
			det,
			"/tmp/trickplay",
		)
		item := &domainlibrary.MediaItem{
			ID:   shared.MediaItemID("ep-1"),
			Type: domainlibrary.Episode,
			File: &domainlibrary.MediaFile{Path: "/tmp/ep.mkv"},
		}

		segs, err := svc.runPipeline(context.Background(), item, nil)
		require.NoError(t, err)

		assert.Equal(t, int32(0), det.leadingLogoCalls.Load(), "the pass must never run on an episode")
		for _, seg := range segs {
			assert.NotEqual(t, "leading_black", seg.Source, "no leading-logo segment must reach an episode's result")
		}
	})
}

// A higher-authority pass that already filled the intro type must pre-empt
// the leading-logo-block pass, like every other intro-only pass in the
// pipeline (needIntro()).
func TestRunPipeline_LeadingLogoBlock_SkippedWhenIntroAlreadyFilled(t *testing.T) {
	t.Parallel()
	det := &countingDetector{fakeDetector: fakeDetector{
		leadingLogoFound: true, leadingLogoStartMs: 0, leadingLogoEndMs: 35_000,
	}}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{},
		&fakeConfigRepo{},
		det,
		"/tmp/trickplay",
	)
	hint := &domainlibrary.MediaSegment{
		Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "audio_cluster", Confidence: 0.9,
	}
	item := &domainlibrary.MediaItem{
		ID:   shared.MediaItemID("movie-2"),
		Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{Path: "/tmp/movie.mkv"},
	}

	// A season-cluster hint is authored above the leading-logo pass in the
	// pipeline order and fills "intro" first.
	segs, err := svc.runPipeline(context.Background(), item, hint)
	require.NoError(t, err)

	assert.Equal(t, int32(0), det.leadingLogoCalls.Load(),
		"a higher-authority pass already filled intro, the leading-logo pass must not run")
	require.Len(t, segs, 1)
	assert.Equal(t, "audio_cluster", segs[0].Source)
}

// A community-database answer is never served (IsCommunityIntroSource), so it
// must not pre-empt the one pass that can find a film's intro locally. Measured
// in prod 2026-08-29 (gold dry-run): every film tidb knew about ended with NO
// served intro, because the provider filled "intro" first and the leading-logo
// pass never ran - while the same pass scored IoU 0.96 on the films tidb did
// not know.
func TestRunPipeline_LeadingLogoBlock_RunsBeforeCommunityProvider(t *testing.T) {
	t.Parallel()
	det := &countingDetector{fakeDetector: fakeDetector{
		leadingLogoFound: true, leadingLogoStartMs: 0, leadingLogoEndMs: 35_000,
	}}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{},
		&fakeConfigRepo{},
		det,
		"/tmp/trickplay",
	).WithIntroMetadataProvider(&fakeMetadataProvider{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 39_000, Source: "tidb", Confidence: 0.97},
		{Type: "end_credits", StartMs: 6_588_000, EndMs: 7_315_712, Source: "tidb", Confidence: 0.97},
	}})
	item := &domainlibrary.MediaItem{
		ID:   shared.MediaItemID("movie-3"),
		Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{Path: "/tmp/movie.mkv"},
	}

	segs, err := svc.runPipeline(context.Background(), item, nil)
	require.NoError(t, err)

	require.Equal(t, int32(1), det.leadingLogoCalls.Load(),
		"a community provider answer must not starve the film's local pass")
	byType := map[string]domainlibrary.MediaSegment{}
	for _, s := range segs {
		byType[s.Type] = s
	}
	require.Contains(t, byType, "intro")
	assert.Equal(t, "leading_black", byType["intro"].Source, "the file-derived intro must win over the community row")
	require.Contains(t, byType, "end_credits")
	assert.Equal(t, "tidb", byType["end_credits"].Source, "the provider stays additive for the types the local passes missed")
}
