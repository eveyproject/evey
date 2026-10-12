package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

func stereoAudioFp() AudioFingerprint {
	return AudioFingerprint{
		MediaID: "m1", SourcePath: "/media/film.mkv", SourceTrackIdx: 0,
		AudioCodec: "aac", AudioBitrate: 128, MaxAudioChannels: 2,
	}
}

func stereoDecisionFp() DecisionFingerprint {
	return DecisionFingerprint{
		MediaID: "m1", SourcePath: "/media/film.mkv",
		VideoCodec: "h264", AudioCodec: "aac", MaxAudioChannels: 2,
	}
}

// THE point of the version cran: nothing in either fingerprint's FIELDS
// changes when the matrix policy changes (MaxAudioChannels is 2 before and
// after), so without the policy in the digest the orchestrator would keep
// serving the clipped segments it produced under the old policy and the fix
// would never reach a player.
func TestDownmixPolicyChange_ForksBothCaches(t *testing.T) {
	afp := stereoAudioFp()
	assert.NotEqual(t,
		afp.hashWithDownmixPolicy("rematrix_maxval=1.0000"),
		afp.hashWithDownmixPolicy("rematrix_maxval=1.5233"),
		"audio rendition: a different matrix policy must not alias in the cache")

	dfp := stereoDecisionFp()
	assert.NotEqual(t,
		dfp.hashWithDownmixPolicy("rematrix_maxval=1.0000"),
		dfp.hashWithDownmixPolicy("rematrix_maxval=1.5233"),
		"muxed session: a different matrix policy must not alias in the cache")
}

// The bump actually happened: these are the hashes the pre-fix code produced
// for the very same structs (digest without any "dmx=" line). A stereo session
// MUST have moved off them.
func TestDownmixPolicy_MovedOffTheUnprotectedHashes(t *testing.T) {
	afp := stereoAudioFp()
	require.Equal(t, domaintranscoding.AudioDownmixPolicy(0, 2), afp.downmixPolicy())
	assert.NotEqual(t, afp.hashWithDownmixPolicy(""), afp.Hash(),
		"stereo audio rendition still hashes like the unprotected fold")

	dfp := stereoDecisionFp()
	assert.NotEqual(t, dfp.hashWithDownmixPolicy(""), dfp.Hash(),
		"stereo muxed session still hashes like the unprotected fold")
}

// 2026-09-04: the cap moved 1.0 -> 2.0 to give the fold back the 6 dB the
// normalisation took. Segments already on disk were produced at 1.0 and are 6 dB
// quieter, so they MUST NOT be served for a session that now asks for 2.0 -
// otherwise the level a viewer gets depends on whether someone watched the title
// before them. The policy string carries the cap, so this is automatic; the test
// exists so it stays that way.
func TestDownmixPolicy_LevelChangeForksTheCache(t *testing.T) {
	const fullyNormalised = "rematrix_maxval=1.0000"
	require.NotEqual(t, fullyNormalised, domaintranscoding.AudioDownmixPolicy(0, 2),
		"this test is stale: the cap is back at the fully normalised value")

	afp := stereoAudioFp()
	assert.NotEqual(t, afp.hashWithDownmixPolicy(fullyNormalised), afp.Hash(),
		"audio rendition would serve the 6 dB quieter segments cached under the old cap")

	dfp := stereoDecisionFp()
	assert.NotEqual(t, dfp.hashWithDownmixPolicy(fullyNormalised), dfp.Hash(),
		"muxed session would serve the 6 dB quieter segments cached under the old cap")
}

// The nominal 5.1 path produces the SAME bytes as before, so it must keep its
// cache: invalidating it would re-transcode every multichannel session for
// nothing. Same for the wasm passthrough, which re-encodes nothing at all.
func TestDownmixPolicy_LeavesUntouchedPathsOnTheirHashes(t *testing.T) {
	afp := stereoAudioFp()
	afp.MaxAudioChannels = 6
	assert.Empty(t, afp.downmixPolicy())
	assert.Equal(t, afp.hashWithDownmixPolicy(""), afp.Hash())

	copyFp := stereoAudioFp()
	copyFp.AudioCodec = "copy"
	copyFp.MaxAudioChannels = 0
	assert.Empty(t, copyFp.downmixPolicy())
	assert.Equal(t, copyFp.hashWithDownmixPolicy(""), copyFp.Hash())

	dfp := stereoDecisionFp()
	dfp.MaxAudioChannels = 6
	assert.Empty(t, dfp.downmixPolicy())
	assert.Equal(t, dfp.hashWithDownmixPolicy(""), dfp.Hash())

	copyDfp := stereoDecisionFp()
	copyDfp.AudioCopy = true
	assert.Empty(t, copyDfp.downmixPolicy())
}

// The hash must track the policy the ARG BUILDER will actually use, not a
// hand-maintained version string someone can forget to bump.
func TestDownmixPolicy_IsTheOneTheBuilderEmits(t *testing.T) {
	for _, ch := range []int{0, 2, 6} {
		afp := stereoAudioFp()
		afp.MaxAudioChannels = ch
		assert.Equal(t, domaintranscoding.AudioDownmixPolicy(0, ch), afp.downmixPolicy(), "channels=%d", ch)

		dfp := stereoDecisionFp()
		dfp.MaxAudioChannels = ch
		assert.Equal(t, domaintranscoding.AudioDownmixPolicy(0, ch), dfp.downmixPolicy(), "channels=%d", ch)
	}
}

// A source known to be stereo sums nothing: no policy, so its segments keep
// the hash they had before any fold policy existed. An unknown source (0)
// keeps the guard - and the forked hash - because it may be a 5.1.
func TestDownmixPolicy_KnownStereoSourceKeepsItsHash(t *testing.T) {
	afp := stereoAudioFp()
	afp.SourceAudioChannels = 2
	assert.Empty(t, afp.downmixPolicy())
	assert.Equal(t, afp.hashWithDownmixPolicy(""), afp.Hash())

	dfp := stereoDecisionFp()
	dfp.SourceAudioChannels = 2
	assert.Empty(t, dfp.downmixPolicy())
	assert.Equal(t, dfp.hashWithDownmixPolicy(""), dfp.Hash())

	for _, src := range []int{0, 6, 8} {
		a := stereoAudioFp()
		a.SourceAudioChannels = src
		assert.NotEmpty(t, a.downmixPolicy(), "source=%d", src)
		assert.NotEqual(t, a.hashWithDownmixPolicy(""), a.Hash(), "source=%d", src)
	}
}
