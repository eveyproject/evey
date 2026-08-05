package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

type fakeSegmentQueryRepo struct {
	segs []*domainlibrary.MediaSegment
}

func (f *fakeSegmentQueryRepo) GetByMediaItemID(_ context.Context, _ shared.MediaItemID) ([]*domainlibrary.MediaSegment, error) {
	return f.segs, nil
}

func (f *fakeSegmentQueryRepo) GetByMediaItemIDs(_ context.Context, _ []shared.MediaItemID) (map[shared.MediaItemID][]*domainlibrary.MediaSegment, error) {
	return nil, nil
}

func segQuerySvc(segs []*domainlibrary.MediaSegment, opts ...QueryServiceOption) *QueryService {
	opts = append([]QueryServiceOption{WithSegments(&fakeSegmentQueryRepo{segs: segs})}, opts...)
	return NewQueryService(nil, nil, nil, nil, opts...)
}

// The serving gate withholds low-confidence segments from players: a stored
// coin-flip detection (the silence fallback at 0.5) must not surface as a
// skip button, while every measured tier keeps flowing.
func TestGetSegments_WithholdsLowConfidence(t *testing.T) {
	t.Parallel()
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "audio_cluster", Confidence: 0.9},
		{Type: "intro", StartMs: 0, EndMs: 30_000, Source: "audio", Confidence: 0.5},
		{Type: "end_credits", StartMs: 2_400_000, EndMs: 2_580_000, Source: "blackdetect", Confidence: 0.6},
	}
	got, err := segQuerySvc(segs).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, sg := range got {
		assert.GreaterOrEqual(t, sg.Confidence, 0.55)
	}
}

// min_serve_confidence: 0 disables the gate entirely - an admin can opt back
// into serving everything.
func TestGetSegments_ZeroThresholdServesEverything(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"min_serve_confidence": 0}`))
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 30_000, Source: "audio", Confidence: 0.5},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// The threshold is admin-tunable upward too: raising it withholds tiers that
// the default lets through.
func TestGetSegments_RaisedThresholdWithholdsMore(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"min_serve_confidence": 0.8}`))
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "audio_cluster", Confidence: 0.9},
		{Type: "end_credits", StartMs: 2_400_000, EndMs: 2_580_000, Source: "blackdetect", Confidence: 0.6},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "audio_cluster", got[0].Source)
}
