package transcoding

import "fmt"

// audio_downmix.go owns ONE decision: the margin the server gives the channel
// matrix when it folds a multichannel bed down to stereo.
//
// Why it is needed. Folding SUMS channels: swr's 5.1 -> 2.0 matrix is
// L = FL + clev*FC + slev*BL. swr normalises that matrix only when the
// destination sample format is an integer one; feeding a float encoder (aac,
// libopus...) it does NOT, so a bed mastered near full scale lands above 1.0
// and the client's output stage hard-clips it. The centre channel carries
// dialogue at a low level and never sums with anything, which is why voices
// stay clean while music and effects tear - the exact symptom reported.
//
// What rematrix_maxval does. It caps the SUM of the matrix coefficients: swr
// scales every coefficient by maxval/sum when the sum exceeds it. So maxval IS
// the worst-case output for full-scale coherent input - and 1.0 is therefore
// the ONLY value that cannot clip. This is not a taste call, it was measured:
// a first attempt at maxval=1.5233 ("4 dB of headroom", derived from the
// generic 5.1 matrix sum of 1 + 2*0.7071 = 2.4142) still clipped 33 samples on
// a real AC-3 5.1 track, because an AC-3 stream declares its OWN centre and
// surround mix levels and its matrix sums to something else entirely. A fixed
// maxval above 1.0 therefore buys a source-DEPENDENT margin, which is exactly
// the kind of "usually fine" that produced this bug.
//
// Measured through this project's own argv (ffmpeg 8.1.2), 5.1 AC-3 bed at
// -1 dBFS per channel, 12 s, folded to stereo AAC and decoded back to float
// (see audio_downmix_integration_test.go, which re-measures both on demand):
//
//	path                  before                     after
//	audio HLS rendition   +2.67 dBFS, 816 clipped    -2.12 dBFS, 0 clipped
//	compat download       +2.72 dBFS, 2269 clipped   -2.08 dBFS, 0 clipped
//
// The price, stated plainly. The fold comes out quieter by the matrix sum:
// 6.4 dB measured on an AC-3 5.1 track (-2.9 -> -9.3 LUFS), 7.66 dB on a
// lossless 5.1 that declares no mix levels. The listener WILL reach for the
// volume knob on multichannel titles. It is also exactly what every AC-3
// decoder and AV receiver does, it is one static gain - the mix balance and the
// dialogue-to-effects ratio are preserved sample for sample - and it replaces
// 1.9% to 11.9% of samples of hard clipping, depending on the bed.
//
// A limiter was measured as a way to buy the level back and REJECTED:
// alimiter=limit=0.95:level=disabled is not transparent below its threshold
// (it pulled a -0.70 dBFS peak down to -4.39 dBFS, i.e. 3.7 dB BELOW the
// unlimited chain), it left 991 clipped samples in one of the chains tried,
// and it compresses effects against dialogue - an audible change to the mix,
// bought for less level than the static scale already gives.
const (
	// DownmixRematrixMaxval is the cap handed to swr. 1.0 = the fold can reach
	// full scale and no further, whatever the source layout or its declared mix
	// levels.
	DownmixRematrixMaxval = 1.0

	// DefaultStereoChannels is the channel count the encoders fall back to when
	// no ceiling was resolved. Zero has always meant "-ac 2" on this path; the
	// rule lives here so the arg builders and the fold policy cannot drift.
	DefaultStereoChannels = 2
)

// ResolvedAudioChannels is the channel count the encoder is actually told to
// produce for a given ceiling. 0 (nothing resolved) means stereo.
func ResolvedAudioChannels(maxChannels int) int {
	if maxChannels > 0 {
		return maxChannels
	}
	return DefaultStereoChannels
}

// AudioDownmixPolicy is the canonical, hashable description of the fold policy
// applied to an encode targeting maxChannels. Empty when the encode does not
// fold (a 5.1 target keeps its layout, so nothing sums and nothing is emitted).
//
// It is what the segment-cache fingerprints hash, so ANY change to the policy -
// the maxval, the gating, the option itself - forks the cache instead of
// serving segments produced under the previous one. Nothing else in the
// fingerprints covers this: MaxAudioChannels alone is 2 before AND after.
func AudioDownmixPolicy(maxChannels int) string {
	if ResolvedAudioChannels(maxChannels) > DefaultStereoChannels {
		return ""
	}
	return "rematrix_maxval=" + downmixMaxvalArg()
}

// AudioDownmixArgs returns the ffmpeg OUTPUT options that bound the fold, or
// nil when the encode does not fold. Never a filter: this is a resampler
// option, so it costs no filter-graph node and no CPU, and it is inert by
// construction on any matrix whose coefficients already sum below the cap
// (stereo -> stereo, mono -> stereo).
//
// Callers must pass the SAME ceiling they hand resolveAudioChannels, and must
// not call this on a stream copy (there is no resampler to configure).
func AudioDownmixArgs(maxChannels int) []string {
	if AudioDownmixPolicy(maxChannels) == "" {
		return nil
	}
	return []string{"-rematrix_maxval", downmixMaxvalArg()}
}

func downmixMaxvalArg() string {
	return fmt.Sprintf("%.4f", DownmixRematrixMaxval)
}
