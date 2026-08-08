package loadtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A verdict is only readable if the run can name what it decoded, so the mix is
// counted as titles are handed out, not guessed from the pool.
func TestCandidatePool_SourceMix_CountsWhatWasActuallyServed(t *testing.T) {
	t.Parallel()
	p := newCandidatePool([]MediaCandidate{
		{ID: "a", DurationSec: 7200, VideoCodec: "h264", VideoHeight: 1080},
		{ID: "b", DurationSec: 7200, VideoCodec: "h264", VideoHeight: 1080},
		{ID: "c", DurationSec: 7200, VideoCodec: "hevc", VideoHeight: 2160},
	}, time.Hour)

	for range 3 {
		_, ok := p.take()
		require.True(t, ok)
	}

	mix := p.sourceMix()
	require.Len(t, mix, 2)
	assert.Equal(t, SourceMixEntry{Codec: "h264", Height: 1080, Count: 2}, mix[0], "heaviest class first")
	assert.Equal(t, SourceMixEntry{Codec: "hevc", Height: 2160, Count: 1}, mix[1])
}

// Nothing handed out, nothing claimed.
func TestCandidatePool_SourceMix_EmptyBeforeAnyHandout(t *testing.T) {
	t.Parallel()
	p := newCandidatePool([]MediaCandidate{
		{ID: "a", DurationSec: 7200, VideoCodec: "h264", VideoHeight: 1080},
	}, time.Hour)
	assert.Empty(t, p.sourceMix())
}

// A title retired mid-run was never served, so it must not appear in the mix -
// otherwise the verdict claims a source it never decoded.
func TestCandidatePool_SourceMix_IneligibleTitleNeverCounted(t *testing.T) {
	t.Parallel()
	p := newCandidatePool([]MediaCandidate{
		{ID: "dead", DurationSec: 7200, VideoCodec: "hevc", VideoHeight: 2160},
		{ID: "live", DurationSec: 7200, VideoCodec: "h264", VideoHeight: 1080},
	}, time.Hour)
	p.markIneligible("dead")

	_, ok := p.take()
	require.True(t, ok)

	mix := p.sourceMix()
	require.Len(t, mix, 1)
	assert.Equal(t, "h264", mix[0].Codec)
}

// An unprobed title still counts, under an empty codec: the report says how
// much of the run it could not name rather than quietly dropping it.
func TestCandidatePool_SourceMix_UnprobedTitleStillCounted(t *testing.T) {
	t.Parallel()
	p := newCandidatePool([]MediaCandidate{
		{ID: "unprobed", DurationSec: 7200},
	}, time.Hour)

	_, ok := p.take()
	require.True(t, ok)

	mix := p.sourceMix()
	require.Len(t, mix, 1)
	assert.Empty(t, mix[0].Codec)
	assert.Equal(t, 1, mix[0].Count)
}
