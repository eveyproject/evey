package transcoding

// CopySegment is one segment of a video-COPY (remux) rendition: a half-open
// presentation-time range [StartMs, EndMs) that begins exactly on a source
// keyframe. A -c:v copy stream cannot be re-keyed, so every copy segment must
// start at an existing keyframe — these boundaries are computed from the source
// keyframe index, never from a fixed segment duration.
type CopySegment struct {
	StartMs int64
	EndMs   int64
}

// DurationMs is the segment's wall-clock length (its #EXTINF value).
func (s CopySegment) DurationMs() int64 { return s.EndMs - s.StartMs }

// KeyframeSegments partitions [0, durationMs) into copy-rung segments aligned to
// the source keyframes. Each segment starts on a keyframe and is the shortest
// keyframe-aligned span that reaches at least targetMs (so a long-GOP source
// yields longer segments rather than splitting mid-GOP); the final segment runs
// to durationMs. The function is PURE and DETERMINISTIC in
// (keyframes, targetMs, durationMs) — the addressable cache and the precalculated
// manifest both derive segment N's boundary from it, so any drift between the two
// is a permanent cache miss.
//
// keyframes must be sorted ascending (the prober guarantees it). A leading
// keyframe later than 0 is treated as if the first GOP starts at 0 (some muxers
// omit a cue at exactly 0). Keyframes at or beyond durationMs are ignored.
// Returns nil when there is nothing to segment.
func KeyframeSegments(keyframes []int64, targetMs, durationMs int64) []CopySegment {
	if durationMs <= 0 || len(keyframes) == 0 {
		return nil
	}
	if targetMs <= 0 {
		targetMs = 6000
	}

	// Boundary keyframes: 0 first, then every in-range keyframe > 0, ascending.
	bounds := make([]int64, 0, len(keyframes)+1)
	bounds = append(bounds, 0)
	for _, kf := range keyframes {
		if kf > 0 && kf < durationMs {
			bounds = append(bounds, kf)
		}
	}
	// Dedup adjacent (a real cue at exactly 0 collides with the synthetic 0).
	dedup := bounds[:1]
	for _, b := range bounds[1:] {
		if b != dedup[len(dedup)-1] {
			dedup = append(dedup, b)
		}
	}
	bounds = dedup

	segs := make([]CopySegment, 0, len(bounds))
	start := bounds[0]
	for _, b := range bounds[1:] {
		if b-start >= targetMs {
			segs = append(segs, CopySegment{StartMs: start, EndMs: b})
			start = b
		}
	}
	// The tail from the last boundary to the end of the media is the final segment.
	if start < durationMs {
		segs = append(segs, CopySegment{StartMs: start, EndMs: durationMs})
	}
	return segs
}
