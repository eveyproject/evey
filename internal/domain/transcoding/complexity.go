package transcoding

import "context"

// ComplexityMetrics is the raw signal we extract from a media file at
// classify time. Stored alongside the class so the admin can reproduce or
// override the decision after the fact, and so a future classifier rev
// can be backfilled without re-running ffmpeg on the source.
type ComplexityMetrics struct {
	// MotionMean averages ffmpeg signalstats `YDIF` - frame-to-frame
	// luminance differences - across the sampled frames. Proxy for
	// temporal complexity: low ⇒ static scenes (dialogue, anime
	// cell-shading), high ⇒ fast cuts / action / camera movement.
	// Range typically 0..30 on real content. (The field was originally
	// named YDEVMean but signalstats never emitted YDEV - the regex
	// silently produced zeros for every classify. Switched to YDIF
	// which signalstats does emit.)
	MotionMean float64 `json:"motion_mean"`

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

	// FramesSampled tells the admin how many frames went into the
	// MotionMean average - lets them spot a degenerate "we only sampled 3
	// frames" classification.
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

	// Sample-free fallback: when the per-frame YDEV / scenecut pipeline
	// returned nothing (ffmpeg killed, file too short, unsupported codec)
	// the YDEV / Scenecuts are zero and the cut-based heuristics below
	// would all match those zeros and bucket everything as low-motion.
	// Decide on bitrate + resolution alone instead - coarser but always
	// safer than a wrong ladder pick.
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

	// Thresholds for MotionMean (signalstats YDIF) calibrated against
	// the typical 0..30 range: < 1.5 ≈ static, 2-5 ≈ normal motion,
	// > 5 ≈ sustained motion. Old (0..127 YDEV) thresholds were 25 / 40
	// / 50 / 55 - rebalanced below.

	// Sustained motion + many cuts → sport. Rare on a film/TV library
	// but cheap to gate. Must come before high-motion so we don't shadow.
	if m.ScenecutsPerMinute > 20 && m.MotionMean > 4 {
		return ClassSport
	}

	// Animation heuristic: low motion + few hard cuts + source encoder
	// already got away with low bitrate. All three matter - a low-bitrate
	// live-action rip from 2003 also has low motion but doesn't belong
	// on the anim ladder.
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
