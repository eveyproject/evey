package library

import (
	"context"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// The cluster window is served as is: the silence refinement collapsed Monarch S02E02
// to <1s (intro lost), the subtitle vote pulled Spider-Noir 16s early (2026-09-27).
func TestRunPipeline_ClusterHintServedUntouched(t *testing.T) {
	t.Parallel()
	svc := NewIntroDetectionService(
		&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{},
		&fakeDetector{
			precisionStartMs: 102_000, precisionEndMs: 102_900, // a collapsing silence pair
			subtitleCues: []int64{95_000},
			sceneCuts:    []int64{104_000},
		},
		"/tmp/trickplay",
	)
	hint := &domainlibrary.MediaSegment{Type: "intro", StartMs: 80_000, EndMs: 107_000, Source: "audio_cluster", Confidence: 0.9}
	item := &domainlibrary.MediaItem{ID: shared.MediaItemID("ep-1"), Type: domainlibrary.Episode, File: &domainlibrary.MediaFile{Path: "/tmp/x.mkv"}}

	segs, err := svc.runPipeline(context.Background(), item, hint)
	require.NoError(t, err)
	var intro *domainlibrary.MediaSegment
	for i := range segs {
		if segs[i].Type == "intro" {
			intro = &segs[i]
		}
	}
	require.NotNil(t, intro)
	require.Equal(t, int64(80_000), intro.StartMs)
	require.Equal(t, int64(107_000), intro.EndMs)
	require.InDelta(t, 0.8, intro.Confidence, 1e-9, "a lone audio witness, below the 0.9 the season prior treats as corroborated")
}

func TestGapRunAtShift_BridgesShortGapsOnly(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(7))
	a := makeNoise(r, 400)
	b := makeNoise(r, 400)
	copy(b[100:150], a[100:150]) // shared 100-150
	copy(b[160:200], a[160:200]) // 10-frame gap: bridged (gap limit 28)
	copy(b[260:300], a[260:300]) // 60-frame gap: a separate, shorter run
	s, e := gapRunAtShift(a, b, 0, 6, 28)
	require.Equal(t, 100, s)
	require.Equal(t, 200, e)
}

// A remix breaks the strict run mid-intro: the signature covers only part of it, and a
// neighbour sharing the whole intro widens this carrier. A match elsewhere in the
// episode (a recurring cue) does not touch the window.
func TestExtendCarrierWindow_WidensFromNeighboursNeverMoves(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(11))
	intro := makeNoise(r, 200)
	ep := func(at int) []uint32 {
		fp := makeNoise(r, 900)
		copy(fp[at:], intro)
		return fp
	}
	e1, e2 := ep(100), ep(300)
	cue := makeNoise(r, 40)
	copy(e1[600:], cue)
	copy(e2[700:], cue)
	eps := []episodeFP{{id: "e1", fp: e1}, {id: "e2", fp: e2}}
	cl := seasonCluster{
		sig:      e1[150:210], // the strict run found only 60 of the 200 intro frames
		carriers: []clusterCarrier{{id: "e1", offsetFrames: 150}, {id: "e2", offsetFrames: 350}},
	}
	s, e := extendCarrierWindow(eps, cl, cl.carriers[0])
	require.Equal(t, 100, s)
	require.Equal(t, 300, e)
}
