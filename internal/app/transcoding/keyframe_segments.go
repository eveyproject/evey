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
// boundary at EVERY source keyframe - one segment per keyframe gap, no
// coalescing. The function is PURE and DETERMINISTIC in (keyframes, durationMs):
// the addressable cache and the precalculated manifest both derive segment N's
// boundary from it, so any drift between the two is a permanent cache miss.
//
// WHY every keyframe (no minimum-duration merging): the ffmpeg HLS muxer is the
// other half of this contract - the copy session runs `-hls_time 0` so it also
// splits at every keyframe. That is the ONLY split rule ffmpeg can apply that is
// independent of where the session started. With any hls_time > 0 the muxer
// splits on an ABSOLUTE n×hls_time grid relative to the session's own start
// (NOT "duration since the previous cut"): on long-GOP sources the grid lags
// behind the cuts until every keyframe qualifies, and a `-ss` session's grid is
// offset from the cold session's. Measured in prod (Doctor Strange, H.264
// ~10.4 s GOPs, -hls_time 6): ffmpeg cut at every keyframe while the plan
// coalesced to ≥6 s spans → the segment NUMBERING drifted minutes from the
// playlist (segment 226 carried content for t=2021 s where the playlist said
// t=2216 s). Reproduced and verified against jellyfin-ffmpeg7: `-hls_time 0`
// splits at every keyframe for cold AND seek sessions alike, which a
// keyframe-list plan replicates exactly. Pathologically dense keyframes
// (all-intra sources) are rejected upstream by the planner's density guard.
//
// keyframes must be sorted ascending (the prober guarantees it). Keyframes at
// or beyond durationMs are ignored. Returns nil when there is nothing to
// segment.
//
// THE FIRST KEYFRAME IS BOUNDARY 0, it is not a cut. ffmpeg at `-hls_time 0`
// opens segment 0 on the first packet it muxes and cuts on every keyframe AFTER
// that: it never emits a segment for the leading partial GOP. So a source whose
// first keyframe sits at 44 ms (no cue at exactly 0 - common) yields the SAME
// segment count and numbering as one with a cue at 0; only the produced
// timeline's origin moves by those 44 ms, which is what the rebase pump's
// tolerance absorbs. Emitting both a synthetic 0 bound and that first keyframe
// invented a phantom [0,44 ms) segment ffmpeg never produced and shifted the
// whole plan one index against the real output (prod: every copy segment
// tripping the divergence tripwire, and a manifest whose time<->segment map was
// a full GOP off for the entire media).
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
