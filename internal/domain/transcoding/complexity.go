package transcoding

import "context"

// ComplexityMetrics is the raw signal we extract from a media file at
// classify time. Stored alongside the class so the admin can reproduce or
// override the decision after the fact, and so a future classifier rev
// can be backfilled without re-running ffmpeg on the source.
type ComplexityMetrics struct {
	// YDEV is the average per-frame Y plane standard deviation across the
	// sampled frames. Proxy for spatial detail — high YDEV ⇒ complex frame
	// (grain, sharp edges, dense texture), low YDEV ⇒ flat surfaces (anime,
	// animated titles). Range 0..127 typically.
	YDEVMean float64 `json:"ydev_mean"`

	// ScenecutsPerMinute counts frames flagged by ffmpeg's scene-detection
	// filter (`select='gt(scene,0.3)'`) over a minute of sampled content.
	// High value ⇒ sustained action / fast cuts (action / sport). Low ⇒
	// dialogue, static shots.
	ScenecutsPerMinute float64 `json:"scenecuts_per_minute"`

	// SourceBitrateKbps is the input file's overall bitrate, in kbps,
	// straight from ffprobe `format.bit_rate / 1000`. Tells us how the
	// source encoder valued the content; very low ⇒ already compressed
	// efficiently (anime, broadcast TV), very high ⇒ untouched UHD master.
	SourceBitrateKbps int64 `json:"source_bitrate_kbps"`

	// SourceWidth / SourceHeight from the video stream. Used by the
	// classifier (4K HDR action capping rule) and by CapLadderToSource at
	// playback time.
	SourceWidth  int `json:"source_width"`
	SourceHeight int `json:"source_height"`

	// FramesSampled tells the admin how many frames went into the YDEV
	// average — lets them spot a degenerate "we only sampled 3 frames"
	// classification.
	FramesSampled int `json:"frames_sampled"`
}

// Classify returns the ContentClass that best matches the metrics, using a
// heuristic tuned on the Mux / Netflix per-title blog numbers. Decision
// table (top match wins; LadderFilmNormal is the safe default):
//
//	source ≥ 2160p AND bitrate ≥ 15 Mbps           → film_high_motion
//	source ≥ 2160p (any bitrate)                    → film_normal (refuse to call 4K animation: too risky on grain)
//	YDEV < 25 AND scenecuts < 6/min AND bitrate < 3 Mbps → animation
//	YDEV < 40 AND scenecuts < 8/min                → film_low_motion
//	scenecuts > 20/min AND YDEV > 50               → sport
//	YDEV > 55 OR scenecuts > 15/min                → film_high_motion
//	default                                         → film_normal
//
// The thresholds are empirical starting points from the literature; the
// admin can reclassify any title manually if a misfire shows up.
func Classify(m ComplexityMetrics) ContentClass {
	// 4K territory: very few "anime in 4K" with low complexity exist; the
	// downside of misclassifying a real 4K master as animation (3.2 Mbps
	// top rung instead of 15 Mbps) is catastrophic to perceived quality.
	// Bias to high-motion for high-bitrate 4K, normal otherwise.
	if m.SourceHeight >= 2160 {
		if m.SourceBitrateKbps >= 15000 {
			return ClassFilmHighMotion
		}
		return ClassFilmNormal
	}

	// Sustained motion + complex frames → sport. Rare on a film/TV library
	// but cheap to gate. Must come before high-motion so we don't shadow it.
	if m.ScenecutsPerMinute > 20 && m.YDEVMean > 50 {
		return ClassSport
	}

	// Animation heuristic: flat textures (low YDEV), few hard cuts, and the
	// source encoder already got away with a low bitrate. All three matter —
	// a low-bitrate live-action rip from 2003 also has low YDEV but doesn't
	// belong on the anim ladder.
	if m.YDEVMean < 25 && m.ScenecutsPerMinute < 6 && m.SourceBitrateKbps < 3000 {
		return ClassAnimation
	}

	if m.YDEVMean < 40 && m.ScenecutsPerMinute < 8 {
		return ClassFilmLowMotion
	}

	if m.YDEVMean > 55 || m.ScenecutsPerMinute > 15 {
		return ClassFilmHighMotion
	}

	return ClassFilmNormal
}

// Classifier extracts ComplexityMetrics from a media file by sampling
// frames via ffmpeg signalstats and the scene-detection filter. Adapter
// implementation lives in the ffmpeg driven adapter; tests use a stub.
type Classifier interface {
	// Classify runs the metric pipeline on the file at filePath and
	// returns the metrics. The caller maps metrics → ContentClass via
	// Classify(). Returns an error only on hard failures (file unreadable,
	// ffmpeg crash); a partial result with FramesSampled > 0 still gets
	// returned alongside a nil error so the admin gets visibility on
	// degenerate cases (3-second sample because the file is short, etc.).
	Classify(ctx context.Context, filePath string, durationSec float64) (ComplexityMetrics, error)
}
