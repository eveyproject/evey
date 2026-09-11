package transcoding

import "fmt"

// audio_downmix.go owns ONE decision: the headroom swr gets when it folds a
// multichannel bed to stereo. swr only normalises the fold matrix for integer
// sample formats; into a float encoder (aac, libopus) a full-scale bed sums
// above 1.0 and clips (dialogue in the centre stays clean, music tears).
// rematrix_maxval caps the coefficient SUM: 1.0 is the only value that cannot
// clip, but it made this server the one link that never gives the level back
// (fold 6.4-7.7 dB quieter than a browser folding the same 5.1). 2.0 is the
// reference player's level to the tenth of a LU (-23.0 LUFS, 2026-09-04) and
// keeps 1.66 dB under the browser; balance is untouched (global scale). A
// limiter was measured and rejected: not transparent below threshold, and its
// look-ahead delays a sample-aligned rendition.
const (
	// DownmixRematrixMaxval is inert on any matrix already summing below it
	// (stereo -> stereo is bit-identical at 1.0 and 2.0); only a real fold scales.
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
