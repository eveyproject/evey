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
