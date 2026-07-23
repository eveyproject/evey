package httpapi

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// Cast progress derivation (user-requested 2026-07-20): once the sender's
// browser closes, the receiver's segment fetches on its CAST token are the
// only playback signal left. The serving handlers call observeCastFetch with
// their best estimate of the fetched media END time; the playback-side
// tracker turns those into watch-progress rows (buffer margin, monotonic,
// throttled). Per-path estimates:
//   - transcode ladder segments: EXACT (fixed 3s grid);
//   - od_ plan segments: proportional idx/total x duration (keyframe-cut
//     around 6s, so grid math would drift - proportional is bounded);
//   - direct play: byte-offset ratio x duration (VBR-biased, still worlds
//     better than the zero reporting it replaces);
//   - ffmpeg COPY VOD rung: NOT observed - its segments cut on source
//     keyframes with no grid, and a grid estimate can overshoot by minutes.

// castFetchObserver is the playback tracker's slice the handlers need.
// Nil = feature not wired (tests, minimal builds) - a no-op.
type CastFetchObserver interface {
	ObserveFetch(profileID, mediaItemID string, endMs int64)
}

// observeCastFetch feeds the tracker when - and only when - the request rode
// a CAST-marked stream token. Local players (short sliding token, or the
// Safari/iOS "native" long token) report their own progress; deriving from
// their fetches too would fight those reports with buffer-ahead estimates.
func observeCastFetch(r *http.Request, obs CastFetchObserver, endMs int64) {
	if obs == nil || endMs <= 0 {
		return
	}
	st, ok := StreamTokenFromContext(r.Context())
	if !ok || st == nil || !st.Cast {
		return
	}
	obs.ObserveFetch(string(st.ProfileID), string(st.MediaItemID), endMs)
}

var onDemandSegNameRE = regexp.MustCompile(`^seg(\d+)\.m4s$`)

// parseRangeStart extracts the first byte offset of a "bytes=START-..." Range
// header. Good enough for progress estimation: multipart ranges and suffix
// ranges ("bytes=-500") return false rather than a wrong guess.
func parseRangeStart(rangeHeader string) (int64, bool) {
	const prefix = "bytes="
	if !strings.HasPrefix(rangeHeader, prefix) {
		return 0, false
	}
	spec := strings.TrimPrefix(rangeHeader, prefix)
	dash := strings.IndexByte(spec, '-')
	if dash <= 0 { // absent, or a suffix range "-500"
		return 0, false
	}
	start, err := strconv.ParseInt(spec[:dash], 10, 64)
	if err != nil || start < 0 {
		return 0, false
	}
	return start, true
}

// onDemandSegEndMs estimates the media END time of an od_ plan segment from
// its CANONICAL name (prefix already stripped). seg00001 is the first
// segment. Returns 0 when no estimate is possible (non-segment resource,
// unknown plan size or duration).
func onDemandSegEndMs(name string, totalSegments int, durMs int64) int64 {
	if totalSegments <= 0 || durMs <= 0 {
		return 0
	}
	m := onDemandSegNameRE.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil || idx <= 0 {
		return 0
	}
	if idx >= totalSegments {
		return durMs
	}
	return durMs * int64(idx) / int64(totalSegments)
}
