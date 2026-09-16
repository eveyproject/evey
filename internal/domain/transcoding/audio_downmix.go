package transcoding

import "fmt"

// audio_downmix.go owns ONE decision: the level policy when a multichannel bed
// is folded to stereo. A fully normalised fold (maxval 1.0) never clips but sits
// 6-7.7 dB under every other player; with makeup a loud mix exceeds full scale
// and the integer encoder input hard-clips it. So the fold runs in float inside
// the filter graph, followed by a look-ahead limiter: bit-identical below the
// ceiling (latency compensated), gain-ridden above.
const (
	// DownmixRematrixMaxval is inert on any matrix already summing below it
	// (stereo -> stereo is bit-identical at 1.0 and 1.41); only a real fold scales.
	// +3 dB, not +6: at 2.0 the limiter worked 16% of a film climax with 7 dB
	// swings (audible pumping); at 1.41 it is 4% and 4 dB.
	DownmixRematrixMaxval = 1.4142

	// DownmixLimiterCeiling is -2 dBFS, the lossy-delivery margin: alimiter
	// caps sample peaks, and an AAC decode overshoots by up to ~1.8 dB on tonal
	// content (measured), which the client's integer output would clip.
	DownmixLimiterCeiling = 0.7943

	// Look-ahead / release in ms. Release is ffmpeg's default: slower only
	// lowered the passage after a peak (50 vs 200 ms measured).
	DownmixLimiterAttackMs  = 5
	DownmixLimiterReleaseMs = 50

	// DefaultStereoChannels is what "no ceiling resolved" has always encoded to.
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

// AudioFolds reports whether an encode from sourceChannels to maxChannels sums
// channels. An unknown source (0) counts as a possible fold: one filter node
// is cheaper than a clipped film.
func AudioFolds(sourceChannels, maxChannels int) bool {
	target := ResolvedAudioChannels(maxChannels)
	if target > DefaultStereoChannels {
		return false
	}
	return sourceChannels <= 0 || sourceChannels > target
}

// AudioDownmixPolicy is the hashable description of the fold policy, empty
// when nothing folds so those sessions keep their cache. The segment-cache
// fingerprints hash it because none of their fields moves when the policy does.
func AudioDownmixPolicy(sourceChannels, maxChannels int) string {
	return AudioFoldFilter(sourceChannels, maxChannels)
}

// AudioFoldFilter is the audio filter chain of the fold, "" when the encode
// does not fold. Callers pass the same ceiling as resolveAudioChannels, join it
// with any other audio filter (ffmpeg keeps only the last -af), and never use
// it on a stream copy.
func AudioFoldFilter(sourceChannels, maxChannels int) string {
	if !AudioFolds(sourceChannels, maxChannels) {
		return ""
	}
	layout := "stereo"
	if ResolvedAudioChannels(maxChannels) == 1 {
		layout = "mono"
	}
	return fmt.Sprintf(
		"aresample=out_chlayout=%s:rematrix_maxval=%.4f,alimiter=limit=%.4f:attack=%d:release=%d:level=0:latency=1",
		layout, DownmixRematrixMaxval, DownmixLimiterCeiling, DownmixLimiterAttackMs, DownmixLimiterReleaseMs)
}

// JoinAudioFilters joins the non-empty parts into one chain, in order.
func JoinAudioFilters(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += ","
		}
		out += p
	}
	return out
}
