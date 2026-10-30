package httpapi

// The variant-playlist pre-warm used to open its rafale at segment 0: a resume
// or an audio switch (a restart mid-film) then paid a GPU encoder nobody read
// (prod 2026-09-09). The web player sends ?startSec= on the master; it is
// remembered per media for the variant fetch that follows, as a hint only. It
// is kept in seconds: each rung turns it into ITS segment (a frame-exact grid,
// a copy rung's keyframe plan, an audio rendition's grid).

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// startHintTTL: the variant fetch follows the master within a round-trip; a
// minute keeps a hint from leaking into a later viewing.
const startHintTTL = time.Minute

// livePositionTTL: the last segment served per decision hash outlives a long
// pause - the init of a rung first asked after one must not fall back to 0.
const livePositionTTL = 6 * time.Hour

type positionHint[T int | float64] struct {
	v  T
	at time.Time
}

// positionHints remembers a position per key: the second a viewer's playback
// starts at (startHintKey), or the last segment served per decision hash. The
// zero value is ready to use.
type positionHints[T int | float64] struct {
	mu sync.Mutex
	m  map[string]positionHint[T]
}

// remember records every master's start, 0 included: a variant fetch with no
// fresh master behind it is a mid-viewing refetch, not a cold start.
func (s *positionHints[T]) remember(key string, v T, now time.Time) {
	s.rememberWithin(key, v, now, startHintTTL)
}

func (s *positionHints[T]) rememberWithin(key string, v T, now time.Time, ttl time.Duration) {
	if key == "" || v < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]positionHint[T])
	}
	for k, h := range s.m {
		if now.Sub(h.at) > ttl {
			delete(s.m, k)
		}
	}
	s.m[key] = positionHint[T]{v: v, at: now}
}

// peek is not a take: an ABR probe fetches two variant playlists for one start.
func (s *positionHints[T]) peek(key string, now time.Time) (T, bool) {
	return s.peekWithin(key, now, startHintTTL)
}

func (s *positionHints[T]) peekWithin(key string, now time.Time, ttl time.Duration) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.m[key]
	if !ok || now.Sub(h.at) > ttl {
		var zero T
		return zero, false
	}
	return h.v, true
}

// startHintKey names one viewer's playback of a media: the hint is where THAT
// playback starts. Keyed by the media alone, a second viewer's master at 0
// replaced the first viewer's resume within the minute, and the first viewer's
// variant then warmed an encoder at 0 nobody read. The profile rides the
// stream token too, so a cast receiver lands on its sender's key.
func startHintKey(r *http.Request, mediaID string) string {
	if r != nil {
		if c := HTTPClaimsFromContext(r.Context()); c != nil && c.ProfileID != "" {
			return c.ProfileID + "\x00" + mediaID
		}
	}
	return mediaID
}

// startSecondsHint reads ?startSec= as the second playback starts at; absent,
// zero, negative or unparsable = no hint.
func startSecondsHint(r *http.Request) (float64, bool) {
	raw := r.URL.Query().Get("startSec")
	if raw == "" {
		return 0, false
	}
	sec, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(sec) || math.IsInf(sec, 0) || sec <= 0 {
		return 0, false
	}
	return sec, true
}

// prewarmSegment: the remembered start as a segment of the rung (segmentAt)
// when it lies inside the presentation, else 0. ok is false when no master
// announced a start within startHintTTL: a variant playlist refetched
// mid-viewing (token refresh after a long pause, level fallback) then warmed
// segment 0 on a rung nobody read (prod 2026-10-12).
func (h *hlsVODServe) prewarmSegment(hintKey string, durationSec float64, segmentAt func(sec float64) int, now time.Time) (int, bool) {
	sec, ok := h.startHints.peek(hintKey, now)
	if !ok {
		return 0, false
	}
	if sec <= 0 || (durationSec > 0 && sec >= durationSec) {
		return 0, true
	}
	return segmentAt(sec), true
}

// copySegmentAt is the keyframe segment of a copy rung's plan holding sec; the
// last one past its end, 0 without a plan.
func copySegmentAt(plan []apptranscoding.CopySegment, sec float64) int {
	if len(plan) == 0 {
		return 0
	}
	ms := int64(sec * 1000)
	i := sort.Search(len(plan), func(i int) bool { return plan[i].EndMs > ms })
	return min(i, len(plan)-1)
}

// initSegment: where an init request ensures its rafale. Every rafale writes the
// same init, so: the fresh master's start, else the last segment served on this
// decision (a rung first asked mid-viewing, after a long pause), else 0.
func (h *hlsVODServe) initSegment(hintKey, fpHash string, durationSec float64, segmentAt func(sec float64) int, now time.Time) int {
	if seg, ok := h.prewarmSegment(hintKey, durationSec, segmentAt, now); ok {
		return seg
	}
	if seg, ok := h.livePositions.peekWithin(fpHash, now, livePositionTTL); ok {
		return seg
	}
	return 0
}
