package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type fakeMetadataProvider struct {
	segs []domainlibrary.MediaSegment
}

func (f *fakeMetadataProvider) GetSegments(_ context.Context, _ *domainlibrary.MediaItem) ([]domainlibrary.MediaSegment, error) {
	return f.segs, nil
}

// A community-DB provider must NOT outrank a file-derived detection for the
// same type: its timestamps describe someone else's copy of the media. The
// provider remains additive for the types the local pipeline did not fill
// (here the recap).
func TestRunPipeline_ClusterHintOutranksProviderIntro(t *testing.T) {
	t.Parallel()
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{},
		&fakeConfigRepo{}, // disables trickplay: the pHash pass is skipped
		&fakeDetector{},
		"/tmp/trickplay",
	).WithIntroMetadataProvider(&fakeMetadataProvider{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60_000, Source: "tidb", Confidence: 0.97},
		{Type: "recap", StartMs: 60_000, EndMs: 90_000, Source: "tidb", Confidence: 0.97},
	}})

	hint := &domainlibrary.MediaSegment{
		Type: "intro", StartMs: 80_000, EndMs: 107_000, Source: "audio_cluster", Confidence: 0.9,
	}
	item := &domainlibrary.MediaItem{
		ID:   shared.MediaItemID("ep-1"),
		Type: domainlibrary.Episode,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
	}

	segs, err := svc.runPipeline(context.Background(), item, hint)
	require.NoError(t, err)

	byType := map[string]domainlibrary.MediaSegment{}
	for _, s := range segs {
		byType[s.Type] = s
	}
	require.Contains(t, byType, "intro")
	require.Equal(t, "audio_cluster", byType["intro"].Source,
		"the file-derived cluster hint must win the intro over the provider")
	require.Contains(t, byType, "recap")
	require.Equal(t, "tidb", byType["recap"].Source,
		"the provider must still fill the types the local pipeline missed")
}

// Without any file-derived result, the provider remains a usable fallback for
// the intro (better a community timestamp than nothing).
func TestRunPipeline_ProviderFillsIntroWhenNoLocalResult(t *testing.T) {
	t.Parallel()
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{},
		&fakeTrickplayRepo{},
		&fakeMediaItemRepo{},
		&fakeConfigRepo{},
		&fakeDetector{},
		"/tmp/trickplay",
	).WithIntroMetadataProvider(&fakeMetadataProvider{segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60_000, Source: "tidb", Confidence: 0.97},
	}})

	item := &domainlibrary.MediaItem{
		ID:   shared.MediaItemID("ep-1"),
		Type: domainlibrary.Episode,
		File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"},
	}

	segs, err := svc.runPipeline(context.Background(), item, nil)
	require.NoError(t, err)
	require.Len(t, segs, 1)
	require.Equal(t, "intro", segs[0].Type)
	require.Equal(t, "tidb", segs[0].Source)
}
