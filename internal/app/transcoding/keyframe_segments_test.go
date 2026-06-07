package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
)

func TestKeyframeSegments_GroupsToTarget(t *testing.T) {
	// Keyframes ~3.3s apart; target 6s → each segment spans 2 GOPs (>= 6s), the
	// last runs to the media end.
	kf := []int64{0, 3253, 6548, 9843, 13138}
	got := apptranscode.KeyframeSegments(kf, 6000, 16000)
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 6548},
		{StartMs: 6548, EndMs: 13138},
		{StartMs: 13138, EndMs: 16000},
	}, got)
	// #EXTINF helper.
	assert.Equal(t, int64(6548), got[0].DurationMs())
}

func TestKeyframeSegments_Deterministic(t *testing.T) {
	kf := []int64{0, 2000, 4000, 6000, 8000, 10000}
	a := apptranscode.KeyframeSegments(kf, 3000, 11000)
	b := apptranscode.KeyframeSegments(kf, 3000, 11000)
	assert.Equal(t, a, b, "same inputs ⇒ byte-identical boundaries (cache-hash invariant)")
	// 3s target, 2s GOPs → segments of 4s (2 GOPs), tail to 11000.
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 4000},
		{StartMs: 4000, EndMs: 8000},
		{StartMs: 8000, EndMs: 11000},
	}, a)
}

func TestKeyframeSegments_SyntheticZeroLeadingKeyframe(t *testing.T) {
	// First cue at 500ms (no cue at exactly 0): the first GOP still starts at 0.
	got := apptranscode.KeyframeSegments([]int64{500, 7000}, 6000, 12000)
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 7000},
		{StartMs: 7000, EndMs: 12000},
	}, got)
}

func TestKeyframeSegments_KeyframesBeyondDurationIgnored(t *testing.T) {
	got := apptranscode.KeyframeSegments([]int64{0, 5000, 99999}, 4000, 8000)
	assert.Equal(t, []apptranscode.CopySegment{
		{StartMs: 0, EndMs: 5000},
		{StartMs: 5000, EndMs: 8000},
	}, got)
}

func TestKeyframeSegments_SingleGOP(t *testing.T) {
	assert.Equal(t, []apptranscode.CopySegment{{StartMs: 0, EndMs: 9000}},
		apptranscode.KeyframeSegments([]int64{0}, 6000, 9000))
}

func TestKeyframeSegments_Edges(t *testing.T) {
	assert.Nil(t, apptranscode.KeyframeSegments(nil, 6000, 10000))
	assert.Nil(t, apptranscode.KeyframeSegments([]int64{0}, 6000, 0))
}
