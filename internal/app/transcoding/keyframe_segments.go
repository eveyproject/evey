package transcoding

// CopySegment is one segment of a video-COPY (remux) rendition: a half-open
// presentation-time range [StartMs, EndMs) that begins exactly on a source
// keyframe. A -c:v copy stream cannot be re-keyed, so every copy segment must
// start at an existing keyframe - these boundaries are computed from the source
// keyframe index, never from a fixed segment duration.
type CopySegment struct {
	StartMs int64
	EndMs   int64
}

// DurationMs is the segment's wall-clock length (its #EXTINF value).
func (s CopySegment) DurationMs() int64 { return s.EndMs - s.StartMs }

// KeyframeSegments partitions [0, durationMs) into copy-rung segments with a
// boundary at EVERY source keyframe, no coalescing. PURE in (keyframes,
// durationMs): the cache and the precalculated manifest both derive segment N
// from it, so any drift is a permanent cache miss.
//
// Every keyframe because the copy session runs `-hls_time 0`, the only ffmpeg
// split rule independent of where the session started (a >0 hls_time splits on
// a grid relative to the session's own start; measured in prod as numbering
// drifting minutes from the playlist on 10 s GOPs). The FIRST keyframe is
// boundary 0, not a cut: ffmpeg opens segment 0 on the first packet and cuts on
// keyframes AFTER it, so a first cue at 44 ms yields the same numbering as one
// at 0 (a synthetic 0 bound invented a phantom segment and shifted the plan by
// one index, prod). keyframes sorted ascending; at/beyond durationMs ignored.
func KeyframeSegments(keyframes []int64, durationMs int64) []CopySegment {
	if durationMs <= 0 || len(keyframes) == 0 {
		return nil
	}

	// Boundaries: 0 (the stream start, which absorbs the first in-range keyframe
	// and any leading partial GOP), then every LATER in-range keyframe, deduped.
	bounds := make([]int64, 0, len(keyframes))
	bounds = append(bounds, 0)
	prev := int64(-1) // last in-range keyframe kept; -1 = none seen yet
	for _, kf := range keyframes {
		if kf < 0 || kf >= durationMs || kf <= prev {
			continue
		}
		if prev >= 0 {
			bounds = append(bounds, kf)
		}
		prev = kf
	}

	segs := make([]CopySegment, 0, len(bounds))
	for i, b := range bounds[1:] {
		segs = append(segs, CopySegment{StartMs: bounds[i], EndMs: b})
	}
	// The tail from the last boundary to the end of the media.
	if last := bounds[len(bounds)-1]; last < durationMs {
		segs = append(segs, CopySegment{StartMs: last, EndMs: durationMs})
	}
	return segs
}
