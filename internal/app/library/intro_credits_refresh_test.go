package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func mustDurationMs(t *testing.T, ms int64) shared.Duration {
	t.Helper()
	d, err := shared.NewDurationMs(ms)
	require.NoError(t, err)
	return d
}

func coveredEpisodeWithCredits(t *testing.T, creditsStartMs int64) (*fakeSegmentRepo, []*domainlibrary.MediaItem) {
	t.Helper()
	idx := 1
	ep := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("ep-1"), Type: domainlibrary.Episode, IndexNumber: &idx,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv", Duration: mustDurationMs(t, 42*60*1000)},
	}
	repo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{MediaItemID: ep.ID, Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "mkv_chapter", Confidence: 1.0},
		{MediaItemID: ep.ID, Type: "end_credits", StartMs: creditsStartMs, EndMs: 42 * 60 * 1000, Source: "blackdetect", Confidence: 0.6},
	}}
	return repo, []*domainlibrary.MediaItem{ep}
}

// A covered episode (intro from a chapter → the sweep skips its pipeline) whose
// blackdetect credits start implausibly early gets its credits re-detected, so
// a credits-parser fix reaches episodes the sweep never re-enters.
func TestDetectSeason_RefreshesSuspectCreditsOnCoveredEpisode(t *testing.T) {
	t.Parallel()
	const durMs = 42 * 60 * 1000
	repo, eps := coveredEpisodeWithCredits(t, durMs-9*60*1000) // 9 min before the end: suspect
	detector := &fakeDetector{creditsFound: true, creditsStartMs: durMs - 80_000, creditsEndMs: durMs}
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, detector, "/tmp/trickplay",
	)

	svc.detectSeason(context.Background(), "season-id")

	require.Len(t, repo.upserted, 1)
	got := repo.upserted[0]
	assert.Equal(t, "end_credits", got.Type)
	assert.Equal(t, int64(durMs-80_000), got.StartMs, "the refreshed credits adopt the re-detected start")
}

// When the fixed detector finds nothing, the suspect row is deleted - no
// credits marker beats a skip prompt appearing mid-scene.
func TestDetectSeason_DeletesSuspectCreditsWhenRefreshFindsNothing(t *testing.T) {
	t.Parallel()
	const durMs = 42 * 60 * 1000
	repo, eps := coveredEpisodeWithCredits(t, durMs-9*60*1000)
	detector := &fakeDetector{creditsFound: false}
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, detector, "/tmp/trickplay",
	)

	svc.detectSeason(context.Background(), "season-id")

	assert.Empty(t, repo.upserted)
	assert.Contains(t, repo.deleted, "ep-1/end_credits")
}

// Plausible credits (close to the end) are left alone - the refresh is a
// targeted repair, not a periodic re-run burning an ffmpeg pass per episode.
func TestDetectSeason_LeavesPlausibleCreditsAlone(t *testing.T) {
	t.Parallel()
	const durMs = 42 * 60 * 1000
	repo, eps := coveredEpisodeWithCredits(t, durMs-90_000) // 90s before the end: plausible
	detector := &fakeDetector{creditsFound: true, creditsStartMs: durMs - 80_000, creditsEndMs: durMs}
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, detector, "/tmp/trickplay",
	)

	svc.detectSeason(context.Background(), "season-id")

	assert.Empty(t, repo.upserted)
	assert.Empty(t, repo.deleted)
}

// The refresh must be self-extinguishing. An irreparable episode - the detector
// keeps answering with a start just as implausible as the stored one - used to
// be re-scanned in full on EVERY sweep, forever, because the refresh wrote the
// new answer without ever checking it was better.
func TestDetectSeason_SuspectCreditsRefreshStopsAfterOneFailedRepair(t *testing.T) {
	t.Parallel()
	const durMs = 42 * 60 * 1000
	repo, eps := coveredEpisodeWithCredits(t, durMs-9*60*1000)
	det := &countingDetector{
		creditsFound: true, creditsStartMs: durMs - 8*60*1000, creditsEndMs: durMs}
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, det, "/tmp/trickplay",
	)

	svc.detectSeason(context.Background(), "season-id")
	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, int(det.creditsCalls.Load()), "the second sweep must not re-pay a full credits scan on an episode the refresh could not repair")
	assert.Empty(t, repo.upserted, "a refreshed start just as implausible as the stored one is not an answer to store")
	assert.Contains(t, repo.deleted, "ep-1/end_credits")
}

// A repaired episode is equally extinct: the plausible row it now carries is no
// longer suspect, so the next sweep skips it without scanning.
func TestDetectSeason_RepairedCreditsAreNotRescanned(t *testing.T) {
	t.Parallel()
	const durMs = 42 * 60 * 1000
	repo, eps := coveredEpisodeWithCredits(t, durMs-9*60*1000)
	det := &countingDetector{
		creditsFound: true, creditsStartMs: durMs - 80_000, creditsEndMs: durMs}
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, det, "/tmp/trickplay",
	)

	svc.detectSeason(context.Background(), "season-id")
	svc.detectSeason(context.Background(), "season-id")

	assert.Equal(t, 1, int(det.creditsCalls.Load()))
	require.Len(t, repo.upserted, 1)
}

// BackfillLibrary must reach MOVIES too: they have no season path, and without
// this a detection/credits fix would never re-touch a film. A movie covered
// only by a community-DB intro re-enters detection (attempt recorded).
func TestBackfillLibrary_CoversMovies(t *testing.T) {
	t.Parallel()
	movie := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("movie-1"), Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{Path: "/tmp/m.mkv", Duration: mustDurationMs(t, 100*60*1000)},
	}
	repo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{MediaItemID: movie.ID, Type: "intro", StartMs: 0, EndMs: 60_000, Source: "tidb", Confidence: 0.97},
	}}
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{item: movie, siblings: []*domainlibrary.MediaItem{movie}},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	n, err := svc.BackfillLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the movie counts as a scheduled unit")
	assert.Equal(t, 1, repo.attempts["movie-1"].count, "the movie entered detection")
}

// A movie whose intro is manual stays untouched by the backfill.
func TestBackfillLibrary_SkipsManualMovie(t *testing.T) {
	t.Parallel()
	movie := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("movie-1"), Type: domainlibrary.MovieItem,
		File: &domainlibrary.MediaFile{Path: "/tmp/m.mkv", Duration: mustDurationMs(t, 100*60*1000)},
	}
	repo := &fakeSegmentRepo{seeded: []*domainlibrary.MediaSegment{
		{MediaItemID: movie.ID, Type: "intro", StartMs: 0, EndMs: 60_000, Source: "manual", Confidence: 1.0},
	}}
	svc := NewIntroDetectionService(
		repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{item: movie, siblings: []*domainlibrary.MediaItem{movie}},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay",
	)

	n, err := svc.BackfillLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, repo.attempts)
}
