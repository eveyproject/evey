package library

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	libraryfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

type fakeIntroMetadataProvider struct {
	segs []domainlibrary.MediaSegment
	err  error
	// calls counts how many times GetSegments was invoked, so the test can
	// assert the pipeline really called it (vs. silently skipping when the
	// adapter is nil).
	calls int
}

func (p *fakeIntroMetadataProvider) GetSegments(_ context.Context, _ *domainlibrary.MediaItem) ([]domainlibrary.MediaSegment, error) {
	p.calls++
	return p.segs, p.err
}

// MKV chapters outrank the provider: they were encoded into THIS file by the
// uploader, while provider timestamps describe someone else's copy of the
// media (the eval measured community DBs tens of seconds off on repacks). We
// pin this by giving the chapterRepo an "Intro" chapter AND a provider intro,
// and asserting the chapter's window wins.
func TestRunPipeline_ChapterPassWinsOverMetadataProvider(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Intro", StartTimeMs: 30_000, EndTimeMs: 60_000},
	}
	svc, item := newServiceWithChapters(t, chapters)

	provider := &fakeIntroMetadataProvider{
		segs: []domainlibrary.MediaSegment{{
			Type:       "intro",
			StartMs:    7_000,
			EndMs:      45_000,
			Source:     "tmdb",
			Confidence: 0.97,
		}},
	}
	svc.WithIntroMetadataProvider(provider)

	got, err := svc.runPipeline(context.Background(), item, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, "mkv_chapter", got[0].Source)
	assert.Equal(t, int64(30_000), got[0].StartMs)
	assert.Equal(t, int64(60_000), got[0].EndMs, "must NOT be 45_000 - the file's own chapter outranks the provider")
}

// When the provider returns nil, the pipeline must fall through to
// chapterPass and return its result. Documents the "no metadata = no harm"
// contract: an adapter without data for an item never blocks detection.
func TestRunPipeline_MetadataProviderEmpty_FallsThroughToChapters(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Intro", StartTimeMs: 30_000, EndTimeMs: 60_000},
	}
	svc, item := newServiceWithChapters(t, chapters)

	provider := &fakeIntroMetadataProvider{segs: nil}
	svc.WithIntroMetadataProvider(provider)

	got, err := svc.runPipeline(context.Background(), item, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, "mkv_chapter", got[0].Source)
}

// Provider errors are non-fatal - pipeline must still try chapters and the
// heuristic chain. This keeps detection robust to a flaky external service
// (HTTP timeout, rate limit, parse error) instead of blocking the whole
// item.
func TestRunPipeline_MetadataProviderError_FallsThroughToChapters(t *testing.T) {
	t.Parallel()
	chapters := []*domainlibrary.Chapter{
		{Title: "Intro", StartTimeMs: 30_000, EndTimeMs: 60_000},
	}
	svc, item := newServiceWithChapters(t, chapters)

	provider := &fakeIntroMetadataProvider{err: errors.New("rate limited by upstream")}
	svc.WithIntroMetadataProvider(provider)

	got, err := svc.runPipeline(context.Background(), item, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, "mkv_chapter", got[0].Source)
}

// Sanity check on the option setter: the pipeline call count must remain at
// 0 when no provider is wired (the conditional branch must not panic on a
// nil interface either).
func TestRunPipeline_NoMetadataProvider_DoesNotCallProvider(t *testing.T) {
	t.Parallel()
	mediaID := shared.MediaItemID("ep-1")
	item := &domainlibrary.MediaItem{
		ID:   mediaID,
		Type: domainlibrary.Episode,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
	}
	chapRepo := libraryfakes.NewFakeChapterRepository()
	require.NoError(t, chapRepo.ReplaceBatch(context.Background(), mediaID, []*domainlibrary.Chapter{
		{Title: "Intro", StartTimeMs: 30_000, EndTimeMs: 60_000},
	}))
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{item: item},
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	)
	svc.WithChapterRepo(chapRepo)
	// Note: intentionally no WithIntroMetadataProvider call.

	got, err := svc.runPipeline(context.Background(), item, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mkv_chapter", got[0].Source)
}
