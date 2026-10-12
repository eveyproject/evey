package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func episodeWithStoredIntro(t *testing.T, source string, attemptVersion int) (*fakeSegmentRepo, []*domainlibrary.MediaItem) {
	t.Helper()
	idx := 1
	ep := &domainlibrary.MediaItem{
		ID: shared.MediaItemID("ep-1"), Type: domainlibrary.Episode, IndexNumber: &idx,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv", Duration: mustDurationMs(t, 42*60*1000)},
	}
	repo := &fakeSegmentRepo{
		seeded: []*domainlibrary.MediaSegment{
			{MediaItemID: ep.ID, Type: "intro", StartMs: 640_000, EndMs: 654_000, Source: source, Confidence: 0.8},
		},
		attempts: map[string]fakeIntroAttempt{"ep-1": {version: attemptVersion, count: 1}},
	}
	return repo, []*domainlibrary.MediaItem{ep}
}

func newVersionTestService(repo *fakeSegmentRepo, eps []*domainlibrary.MediaItem) *IntroDetectionService {
	return NewIntroDetectionService(repo, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps},
		&fakeConfigRepo{}, &fakeDetector{}, "/tmp/trickplay")
}

// A detector intro an older version stored is re-detected once after a bump; when
// this version finds nothing there (a recurring cue it now vetoes), the stale row goes.
func TestDetectSeason_RedetectsAnOlderVersionDetectorIntroAndDropsIt(t *testing.T) {
	t.Parallel()
	repo, eps := episodeWithStoredIntro(t, "audio_cluster", introDetectionAlgoVersion-1)
	newVersionTestService(repo, eps).detectSeason(context.Background(), "season-id")

	assert.Contains(t, repo.deleted, "ep-1/intro", "the older version's intro is not served any more")
	assert.Equal(t, introDetectionAlgoVersion, repo.attempts["ep-1"].version, "the re-detection is recorded at this version")
}

// The same intro detected at the current version is settled: no re-decode.
func TestDetectSeason_SkipsACurrentVersionDetectorIntro(t *testing.T) {
	t.Parallel()
	repo, eps := episodeWithStoredIntro(t, "audio_cluster", introDetectionAlgoVersion)
	newVersionTestService(repo, eps).detectSeason(context.Background(), "season-id")

	assert.NotContains(t, repo.deleted, "ep-1/intro")
	assert.Equal(t, 1, repo.attempts["ep-1"].count, "no new attempt: the episode was not re-detected")
}

// An authored chapter is not the detector's answer: a bump leaves it alone.
func TestDetectSeason_BumpLeavesAChapterIntroAlone(t *testing.T) {
	t.Parallel()
	repo, eps := episodeWithStoredIntro(t, "mkv_chapter", introDetectionAlgoVersion-1)
	newVersionTestService(repo, eps).detectSeason(context.Background(), "season-id")

	assert.NotContains(t, repo.deleted, "ep-1/intro")
	assert.Equal(t, introDetectionAlgoVersion-1, repo.attempts["ep-1"].version, "not re-detected")
}
