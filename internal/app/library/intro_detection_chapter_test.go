package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	libraryfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

func newServiceWithChapters(t *testing.T, chapters []*domainlibrary.Chapter) (*IntroDetectionService, *domainlibrary.MediaItem) {
	t.Helper()
	mediaID := shared.MediaItemID("ep-1")
	item := &domainlibrary.MediaItem{
		ID:   mediaID,
		Type: domainlibrary.Episode,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
	}
	chapRepo := libraryfakes.NewFakeChapterRepository()
	require.NoError(t, chapRepo.ReplaceBatch(context.Background(), mediaID, chapters))
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	)
	svc.WithChapterRepo(chapRepo)
	return svc, item
}

func TestChapterPass_PlainIntroChapter(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Cold Open", StartTimeMs: 0, EndTimeMs: 30_000},
		{Title: "Intro", StartTimeMs: 30_000, EndTimeMs: 60_000},
		{Title: "Episode", StartTimeMs: 60_000, EndTimeMs: 1_800_000},
	}
	svc, item := newServiceWithChapters(t, chapters)
	segs := svc.chapterPass(context.Background(), item)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
	assert.Equal(t, "mkv_chapter", segs[0].Source)
	assert.Equal(t, 1.0, segs[0].Confidence)
	assert.Equal(t, int64(30_000), segs[0].StartMs)
	assert.Equal(t, int64(60_000), segs[0].EndMs)
}

func TestChapterPass_FrenchGenerique(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Générique", StartTimeMs: 0, EndTimeMs: 45_000},
		{Title: "Acte 1", StartTimeMs: 45_000, EndTimeMs: 600_000},
	}
	svc, item := newServiceWithChapters(t, chapters)
	segs := svc.chapterPass(context.Background(), item)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
}

func TestChapterPass_OpeningCredits(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Opening Credits", StartTimeMs: 5_000, EndTimeMs: 50_000},
	}
	svc, item := newServiceWithChapters(t, chapters)
	segs := svc.chapterPass(context.Background(), item)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
}

func TestChapterPass_IntroEndMarker_SynthesizesIntroSegment(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Cold Open", StartTimeMs: 0, EndTimeMs: 25_000},
		{Title: "Intro End", StartTimeMs: 60_000, EndTimeMs: 60_500},
		{Title: "Act 1", StartTimeMs: 60_500, EndTimeMs: 600_000},
	}
	svc, item := newServiceWithChapters(t, chapters)
	segs := svc.chapterPass(context.Background(), item)
	require.Len(t, segs, 1)
	assert.Equal(t, "intro", segs[0].Type)
	assert.Equal(t, int64(0), segs[0].StartMs, "intro spans from previous chapter (Cold Open) to Intro End marker")
	assert.Equal(t, int64(60_000), segs[0].EndMs)
}

func TestChapterPass_IntroEndIgnoredWhenPlainIntroExists(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Intro", StartTimeMs: 5_000, EndTimeMs: 50_000},
		{Title: "Intro End", StartTimeMs: 50_000, EndTimeMs: 50_500},
		{Title: "Episode", StartTimeMs: 50_500, EndTimeMs: 1_800_000},
	}
	svc, item := newServiceWithChapters(t, chapters)
	segs := svc.chapterPass(context.Background(), item)
	// Just one intro from the explicit chapter; the End marker is ignored
	// to avoid duplicate segments.
	require.Len(t, segs, 1)
	assert.Equal(t, int64(5_000), segs[0].StartMs)
	assert.Equal(t, int64(50_000), segs[0].EndMs)
}

func TestChapterPass_RecapAndEndCredits(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Previously", StartTimeMs: 0, EndTimeMs: 60_000},
		{Title: "Intro", StartTimeMs: 60_000, EndTimeMs: 90_000},
		{Title: "Episode", StartTimeMs: 90_000, EndTimeMs: 2_700_000},
		{Title: "End Credits", StartTimeMs: 2_700_000, EndTimeMs: 2_820_000},
	}
	svc, item := newServiceWithChapters(t, chapters)
	segs := svc.chapterPass(context.Background(), item)
	require.Len(t, segs, 3)
	types := map[string]bool{segs[0].Type: true, segs[1].Type: true, segs[2].Type: true}
	assert.True(t, types["intro"])
	assert.True(t, types["recap"])
	assert.True(t, types["end_credits"])
}

func TestChapterPass_NoMatchingChapters_ReturnsNothing(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Chapter 1", StartTimeMs: 0, EndTimeMs: 600_000},
		{Title: "Chapter 2", StartTimeMs: 600_000, EndTimeMs: 1_200_000},
	}
	svc, item := newServiceWithChapters(t, chapters)
	segs := svc.chapterPass(context.Background(), item)
	assert.Empty(t, segs)
}

func TestChapterPass_NoChapterRepo_ReturnsNothing(t *testing.T) {
	t.Parallel()
	item := &domainlibrary.MediaItem{
		ID:   "ep-1",
		Type: domainlibrary.Episode,
	}
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	)
	segs := svc.chapterPass(context.Background(), item)
	assert.Empty(t, segs)
}
