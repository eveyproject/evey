package mkvgo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

func TestConcatCacheKey_DistinguishesChains(t *testing.T) {
	k1 := concatCacheKey([]string{"e1.mkv", "e2.mkv"}, apptranscoding.ConcatOptions{SegmentMs: 6000})
	k2 := concatCacheKey([]string{"e1.mkv", "e3.mkv"}, apptranscoding.ConcatOptions{SegmentMs: 6000})
	k3 := concatCacheKey([]string{"e1.mkv", "e2.mkv", "e3.mkv"}, apptranscoding.ConcatOptions{SegmentMs: 6000})
	assert.NotEqual(t, k1, k2, "a different chain must not alias")
	assert.NotEqual(t, k1, k3, "a longer chain must not alias its prefix")
}

func TestConcatPackager_Plan_RejectsSingleSource(t *testing.T) {
	p := NewConcatPackager()
	defer p.Close()
	_, err := p.Plan(t.Context(), []string{"one.mkv"}, apptranscoding.ConcatOptions{})
	assert.Error(t, err, "concat needs at least two sources - mkvgo's own guard, exercised through the adapter")
}

func fixedDelays(byPath map[string]map[uint64]int64) delaysFunc {
	return func(_ context.Context, path string) (map[uint64]int64, error) {
		d, ok := byPath[path]
		if !ok {
			return nil, errors.New("probe failed")
		}
		return d, nil
	}
}

// The chain is served with the resync each episode gets on its own: every
// source calling for the same shift yields that shift, none yields nothing,
// codec priming under the floor never counts, and a probe failure leaves the
// part without a resync rather than refusing the chain.
func TestConcatAudioShift_AgreeingSources(t *testing.T) {
	ctx := t.Context()
	late := map[uint64]int64{2: 500_000_000}
	shift, err := concatAudioShift(ctx, []string{"e1.mkv", "e2.mkv"}, fixedDelays(map[string]map[uint64]int64{"e1.mkv": late, "e2.mkv": late}))
	require.NoError(t, err)
	assert.Equal(t, late, shift, "the shift every source calls for")

	priming := map[uint64]int64{2: 20_000_000}
	shift, err = concatAudioShift(ctx, []string{"e1.mkv", "e2.mkv"}, fixedDelays(map[string]map[uint64]int64{"e1.mkv": priming, "e2.mkv": nil}))
	require.NoError(t, err)
	assert.Nil(t, shift, "codec priming is not a delay")

	shift, err = concatAudioShift(ctx, []string{"e1.mkv", "e2.mkv"}, fixedDelays(map[string]map[uint64]int64{"e1.mkv": nil}))
	require.NoError(t, err)
	assert.Nil(t, shift, "a failed probe on a part that would need no shift still plans")

	shift, err = concatAudioShift(ctx, []string{"e1.mp4", "e2.mp4"}, fixedDelays(nil))
	require.NoError(t, err)
	assert.Nil(t, shift, "a non-Matroska source is never probed")
}

// Sources that do not call for the same shift refuse the chain: one option
// set serves every part, and the caller falls back to per-episode playback.
func TestConcatAudioShift_DisagreeingSourcesRefuse(t *testing.T) {
	ctx := t.Context()
	late := map[uint64]int64{2: 500_000_000}
	_, err := concatAudioShift(ctx, []string{"e1.mkv", "e2.mkv"}, fixedDelays(map[string]map[uint64]int64{"e1.mkv": nil, "e2.mkv": late}))
	assert.ErrorIs(t, err, ErrConcatAudioShiftMismatch, "one late episode in a chain of aligned ones")

	later := map[uint64]int64{2: 900_000_000}
	_, err = concatAudioShift(ctx, []string{"e1.mkv", "e2.mkv"}, fixedDelays(map[string]map[uint64]int64{"e1.mkv": late, "e2.mkv": later}))
	assert.ErrorIs(t, err, ErrConcatAudioShiftMismatch, "two late episodes by different amounts")

	_, err = concatAudioShift(ctx, []string{"e1.mkv", "e2.mkv"}, fixedDelays(map[string]map[uint64]int64{"e2.mkv": late}))
	assert.ErrorIs(t, err, ErrConcatAudioShiftMismatch, "a failed probe beside a part that needs a shift")
}

// The packager refuses the chain before any source is opened: the sources
// here do not exist, so any other outcome would be a planning error instead.
func TestConcatPackager_Plan_RefusesMismatchedAudioShift(t *testing.T) {
	p := NewConcatPackager()
	defer p.Close()
	p.audioDelays = fixedDelays(map[string]map[uint64]int64{"e1.mkv": nil, "e2.mkv": {2: 500_000_000}})
	_, err := p.Plan(t.Context(), []string{"e1.mkv", "e2.mkv"}, apptranscoding.ConcatOptions{})
	assert.ErrorIs(t, err, ErrConcatAudioShiftMismatch)
}
