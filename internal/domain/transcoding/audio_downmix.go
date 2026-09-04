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
// the ONLY value that cannot clip. That fact still holds and is why any value
// above 1.0 must be chosen on measurement rather than on taste: a first attempt
// at maxval=1.5233 ("4 dB of headroom", derived from the generic 5.1 matrix sum
// of 1 + 2*0.7071 = 2.4142) still clipped 33 samples on a real AC-3 5.1 track,
// because an AC-3 stream declares its OWN centre and surround mix levels and its
// matrix sums to something else entirely. Any maxval above 1.0 therefore buys a
// source-DEPENDENT margin.
//
// The cap sat at 1.0 for that reason from 2026-09-02 to 2026-09-04, and the
// second half of this file explains, with the measurements, why "cannot clip"
// turned out to be the wrong thing to optimise alone: it made the server the one
// link in the chain that never gave the level back. It is 2.0 today. Read the
// DownmixRematrixMaxval block before touching it in either direction.
//
// Measured through this project's own argv (ffmpeg 8.1.2), 5.1 AC-3 bed at
// -1 dBFS per channel, 12 s, folded to stereo AAC and decoded back to float
// (see audio_downmix_integration_test.go, which re-measures both on demand):
//
//	path                  before                     after
//	audio HLS rendition   +2.67 dBFS, 816 clipped    -2.12 dBFS, 0 clipped
//	compat download       +2.72 dBFS, 2269 clipped   -2.08 dBFS, 0 clipped
//
// The price of normalising, stated plainly, and it is what the 2026-09-04 change
// pays back. Fully normalised, the fold comes out quieter by the whole matrix
// sum: 6.4 dB measured on an AC-3 5.1 track (-2.9 -> -9.3 LUFS), 7.66 dB on a
// lossless 5.1 that declares no mix levels. The listener WILL reach for the
// volume knob on multichannel titles - and on a phone that knob ends at the
// hardware maximum, below which nothing more comes out. At 2.0 the fold gives
// that back down to 1.66 dB of the browser's own level, so the knob has room
// again. What never moves, at any cap, is the BALANCE: the scale is global, so
// the mix and the dialogue-to-effects ratio are preserved sample for sample.
//
// A limiter was measured as a way to buy the level back and REJECTED:
// alimiter=limit=0.95:level=disabled is not transparent below its threshold
// (it pulled a -0.70 dBFS peak down to -4.39 dBFS, i.e. 3.7 dB BELOW the
// unlimited chain), it left 991 clipped samples in one of the chains tried,
// and it compresses effects against dialogue - an audible change to the mix,
// bought for less level than the static scale already gives. Re-measured
// 2026-09-04 on this project's own argv: alimiter also DELAYS the signal by its
// look-ahead, which an audio rendition aligned to the sample cannot afford.
//
// 2026-09-04 - why the cap is no longer 1.0. Capping at exactly full scale made
// the fold unable to clip, but it also meant this server was the ONLY player in
// the chain that handed back a quiet stream, and nothing ever returned the level
// the normalisation took. Measured on the same AC-3 5.1 title, same 100 s:
//
//	who folds                       matrix sum   integrated   peak
//	Firefox (web, gets the 5.1)     2.4142       -21.4 LUFS   -0.4 dBFS
//	Jellyfin (-af volume=2)         (2.0)        -23.0 LUFS   -2.1 dBFS
//	Evey, maxval 1.0                1.0          -29.1 LUFS   -8.1 dBFS
//
// So a client that declares a stereo output (every phone and tablet, since the
// real-output-ceiling change) heard the SAME title 6 to 7.7 dB below what the
// very same account got in a browser, which folds the 5.1 itself with the
// unnormalised "speakers" matrix (see web/src/utils/outputChannels.ts). The mix
// balance is untouched by a global scale - the fold is linear, the matrix ratios
// are identical either way - but 7.7 dB is the difference between hearing the
// ambience under the dialogue and losing it below what a tablet speaker puts out
// in a room. Reported as "voices are fine, the background is not on the same
// plane", on mobile only, never on the web. That is this constant.
//
// The measured price, stated as plainly as the gain: the fold can now reach
// twice full scale on a coherent full-scale input, so a peak CAN clip where 1.0
// made it arithmetically impossible. That is the trade every other player in
// this user's chain already makes - Firefox at 2.4142 unbounded, Jellyfin at
// +6.02 dB - and 2.0 keeps 1.66 dB of margin under the browser. It is a
// deliberate reversal of the 1.0 decision, not an oversight of it.
const (
	// DownmixRematrixMaxval is the cap handed to swr: the largest sum the fold's
	// coefficients may reach. It is INERT on any matrix that already sums below
	// it - stereo -> stereo sums to 1, so a stereo source is bit-identical at any
	// value here (verified: -27.077023 dB RMS at both 1.0 and 2.0). Only a real
	// multichannel fold is scaled, which is why the level is given back exactly
	// where it was taken and nowhere else.
	//
	// 2.0 = +6.02 dB over the fully normalised fold, i.e. Jellyfin's level to the
	// tenth of a LU (-23.0 LUFS measured on both).
	DownmixRematrixMaxval = 2.0

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
