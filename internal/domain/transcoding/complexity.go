package transcoding

import "context"

// KnownSourceFacts are the source properties a caller already has, so the
// classifier can skip its ffprobe pre-probe; it probes only what is missing.
type KnownSourceFacts struct {
	BitrateBps int64
	Width      int
	Height     int
}

func (k KnownSourceFacts) Complete() bool {
	return k.BitrateBps > 0 && k.Width > 0 && k.Height > 0
}

// ComplexityMetrics is the raw signal extracted at classify time, stored next
// to the class so the admin can reproduce or override the decision.
type ComplexityMetrics struct {
	// MotionMean averages ffmpeg signalstats YDIF (frame-to-frame luminance
	// difference) over the sampled frames: temporal complexity, typically 0..30.
	MotionMean float64 `json:"motion_mean"`

	// ScenecutsPerMinute counts frames flagged by `select='gt(scene,0.3)'` per
	// minute of sampled content.
	ScenecutsPerMinute float64 `json:"scenecuts_per_minute"`

	// SourceBitrateKbps is the file's overall bitrate: how the source encoder
	// valued the content.
	SourceBitrateKbps int64 `json:"source_bitrate_kbps"`

	SourceWidth  int `json:"source_width"`
	SourceHeight int `json:"source_height"`

	// FramesSampled lets the admin spot a degenerate classification.
	FramesSampled int `json:"frames_sampled"`
	// MetricVersion is the definition the sampled metrics follow. 0/absent =
	// the original run, where MotionMean was signalstats YDIF in the source's
	// NATIVE bit depth (a 10-bit HDR title read ~4x an 8-bit one of the same
	// motion - the defect found 2026-09-12). Version 2 = the canonical 8-bit
	// YDIF8 (see CanonicalYDIF8). The cost model refuses to read a MotionMean
	// below the canonical version, so a native value is never priced silently.
	MetricVersion int `json:"metric_version,omitempty"`
	// Probe is the per-title ground truth (plan P3), when a probe ran.
	Probe *TitleProbe `json:"probe,omitempty"`
}

// CanonicalMetricVersion is the current ComplexityMetrics definition: MotionMean
// is YDIF8 (see CanonicalYDIF8), scdet unchanged.
const CanonicalMetricVersion = 2

// CanonicalYDIF8 converts a native-bit-depth signalstats YDIF to the canonical
// 8-bit scale: YDIF8 = native / 2^(bitDepth-8). signalstats reports the luma
// difference in the frame's own bit depth, so a 10-bit source reads 4x, a
// 12-bit one 16x, an 8-bit one of the same motion. Validated exactly against
// `format=yuv420p,signalstats` on 8- and 10-bit sources (2026-09-12). Depth <=
// 8 (or unknown) is returned unchanged.
func CanonicalYDIF8(nativeYDIF float64, bitDepth int) float64 {
	if bitDepth <= 8 {
		return nativeYDIF
	}
	return nativeYDIF / float64(int64(1)<<(bitDepth-8))
}

// CanonicalMotion reports the MotionMean the model may read, and whether the
// metrics are on the canonical definition at all. Pre-canonical metrics
// (MetricVersion < CanonicalMetricVersion) are refused: their MotionMean is a
// native YDIF that would misprice HDR, so the caller must fall back rather
// than consume it.
func (m ComplexityMetrics) CanonicalMotion() (float64, bool) {
	if m.MetricVersion < CanonicalMetricVersion {
		return 0, false
	}
	return m.MotionMean, true
}

// Classify returns the ContentClass that best matches the metrics, using a
// heuristic tuned on the Mux / Netflix per-title blog numbers. Decision
// table (top match wins; LadderFilmNormal is the safe default):
//
//	source ≥ 2160p AND bitrate ≥ 15 Mbps                      → film_high_motion
//	source ≥ 2160p (any bitrate)                               → film_normal (refuse to call 4K animation: too risky on grain)
//	motion < 1.5 AND scenecuts < 6/min AND bitrate < 3 Mbps    → animation
//	motion < 3 AND scenecuts < 8/min                           → film_low_motion
//	scenecuts > 20/min AND motion > 4                          → sport
//	motion > 5 OR scenecuts > 15/min                           → film_high_motion
//	default                                                    → film_normal
//
// The thresholds are empirical starting points from the literature; the
// admin can reclassify any title manually if a misfire shows up.
func Classify(m ComplexityMetrics) ContentClass {
	// Misclassifying a real 4K master as animation (3.2 Mbps top rung instead
	// of 15) is catastrophic to perceived quality: bias 4K away from it.
	if m.SourceHeight >= 2160 {
		if m.SourceBitrateKbps >= 15000 {
			return ClassFilmHighMotion
		}
		return ClassFilmNormal
	}

	// No sampled frames (ffmpeg killed, short file): the zero metrics would
	// match every low-motion cut below, so decide on bitrate + resolution alone.
	if m.FramesSampled == 0 {
		switch {
		case m.SourceHeight >= 1080 && m.SourceBitrateKbps > 0 && m.SourceBitrateKbps < 2500:
			return ClassAnimation
		case m.SourceHeight >= 1080 && m.SourceBitrateKbps >= 7000:
			return ClassFilmHighMotion
		default:
			return ClassFilmNormal
		}
	}

	// Sport before high-motion so it is not shadowed.
	if m.ScenecutsPerMinute > 20 && m.MotionMean > 4 {
		return ClassSport
	}

	// All three matter: a low-bitrate live-action rip from 2003 also has low
	// motion but does not belong on the animation ladder.
	if m.MotionMean < 1.5 && m.ScenecutsPerMinute < 6 && m.SourceBitrateKbps < 3000 {
		return ClassAnimation
	}

	if m.MotionMean < 3 && m.ScenecutsPerMinute < 8 {
		return ClassFilmLowMotion
	}

	if m.MotionMean > 5 || m.ScenecutsPerMinute > 15 {
		return ClassFilmHighMotion
	}

	return ClassFilmNormal
}

// Classifier extracts ComplexityMetrics from a media file (ffmpeg signalstats
// + scene detection); the ffmpeg driven adapter implements it.
type Classifier interface {
	// Classify returns an error only on hard failures (file unreadable, ffmpeg
	// crash); a partial result with FramesSampled > 0 comes back with a nil
	// error. Every field of known the caller fills is one ffprobe fork saved.
	Classify(ctx context.Context, filePath string, durationSec float64, known KnownSourceFacts) (ComplexityMetrics, error)
}
