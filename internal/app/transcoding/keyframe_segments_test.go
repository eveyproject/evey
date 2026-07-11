package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
)

func TestKeyframeSegments_EveryKeyframeIsABoundary(t *testing.T) {
	// One segment per keyframe gap - NO coalescing, mirroring the copy session's
	// `-hls_time 0` (the only ffmpeg split rule that is session-independent).
	// The last segment runs to the media end.
	kf := []int64{0, 3253, 6548, 9843, 13138}
	got := apptranscode.KeyframeSegments(kf, 16000)
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 3253},
		{StartMs: 3253, EndMs: 6548},
		{StartMs: 6548, EndMs: 9843},
		{StartMs: 9843, EndMs: 13138},
		{StartMs: 13138, EndMs: 16000},
	}, got)
	// #EXTINF helper.
	assert.Equal(t, int64(3253), got[0].DurationMs())
}

func TestKeyframeSegments_IrregularGapsNotMerged(t *testing.T) {
	// Long-GOP source with scene-cut keyframes (10s/3s alternation - the exact
	// prod pattern that exposed the drift). Short gaps MUST stay their own
	// segments: ffmpeg at -hls_time 0 cuts there, so a plan that merged them
	// would shift every later segment's number (the playlist↔content drift bug).
	kf := []int64{0, 10000, 13000, 23000, 26000}
	got := apptranscode.KeyframeSegments(kf, 30000)
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 10000},
		{StartMs: 10000, EndMs: 13000},
		{StartMs: 13000, EndMs: 23000},
		{StartMs: 23000, EndMs: 26000},
		{StartMs: 26000, EndMs: 30000},
	}, got)
}

func TestKeyframeSegments_Deterministic(t *testing.T) {
	kf := []int64{0, 2000, 4000, 6000, 8000, 10000}
	a := apptranscode.KeyframeSegments(kf, 11000)
	b := apptranscode.KeyframeSegments(kf, 11000)
	assert.Equal(t, a, b, "same inputs ⇒ byte-identical boundaries (cache-hash invariant)")
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 2000},
		{StartMs: 2000, EndMs: 4000},
		{StartMs: 4000, EndMs: 6000},
		{StartMs: 6000, EndMs: 8000},
		{StartMs: 8000, EndMs: 10000},
		{StartMs: 10000, EndMs: 11000},
	}, a)
}

func TestKeyframeSegments_SyntheticZeroLeadingKeyframe(t *testing.T) {
	// First cue at 500ms (no cue at exactly 0): the first GOP still starts at 0.
	got := apptranscode.KeyframeSegments([]int64{500, 7000}, 12000)
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 500},
		{StartMs: 500, EndMs: 7000},
		{StartMs: 7000, EndMs: 12000},
	}, got)
}

func TestKeyframeSegments_KeyframesBeyondDurationIgnored(t *testing.T) {
	got := apptranscode.KeyframeSegments([]int64{0, 5000, 99999}, 8000)
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 5000},
		{StartMs: 5000, EndMs: 8000},
	}, got)
}

func TestKeyframeSegments_SingleGOP(t *testing.T) {
	assert.Equal(t, []apptranscode.CopySegment{{StartMs: 0, EndMs: 9000}},
		apptranscode.KeyframeSegments([]int64{0}, 9000))
}

func TestKeyframeSegments_Edges(t *testing.T) {
	assert.Nil(t, apptranscode.KeyframeSegments(nil, 10000))
	assert.Nil(t, apptranscode.KeyframeSegments([]int64{0}, 0))
}
