package httpapi

// The variant-playlist pre-warm used to open its rafale at segment 0: a resume
// or an audio switch (a restart mid-film) then paid a GPU encoder nobody read
// (prod 2026-09-09). The web player sends ?startSec= on the master; it is
// remembered per media for the variant fetch that follows, as a hint only.

import (
	"math"
	"net/http"
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

type startHint struct {
	seg int
	at  time.Time
}

// startPositionHints remembers, per media, the segment playback starts at. The
// zero value is ready to use.
type startPositionHints struct {
	mu sync.Mutex
	m  map[string]startHint
}

// remember records every master's start, 0 included: a variant fetch with no
// fresh master behind it is a mid-viewing refetch, not a cold start.
func (s *startPositionHints) remember(mediaID string, seg int, now time.Time) {
	s.rememberWithin(mediaID, seg, now, startHintTTL)
}

func (s *startPositionHints) rememberWithin(mediaID string, seg int, now time.Time, ttl time.Duration) {
	if mediaID == "" || seg < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]startHint)
	}
	for k, h := range s.m {
		if now.Sub(h.at) > ttl {
			delete(s.m, k)
		}
	}
	s.m[mediaID] = startHint{seg: seg, at: now}
}

// peek is not a take: an ABR probe fetches two variant playlists for one start.
func (s *startPositionHints) peek(mediaID string, now time.Time) (int, bool) {
	return s.peekWithin(mediaID, now, startHintTTL)
}

func (s *startPositionHints) peekWithin(mediaID string, now time.Time, ttl time.Duration) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.m[mediaID]
	if !ok || now.Sub(h.at) > ttl {
		return 0, false
	}
	return h.seg, true
}

// startSegmentHint reads ?startSec= as a segment index; absent, zero, negative
// or unparsable = no hint.
func startSegmentHint(r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("startSec")
	if raw == "" {
		return 0, false
	}
	sec, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(sec) || math.IsInf(sec, 0) || sec <= 0 {
		return 0, false
	}
	seg := int(sec / apptranscoding.ManifestSegmentSeconds)
	if seg <= 0 {
		return 0, false
	}
	return seg, true
}

// prewarmSegment: the remembered start when it lies inside the presentation, else
// 0. ok is false when no master announced a start within startHintTTL: a variant
// playlist refetched mid-viewing (token refresh after a long pause, level
// fallback) then warmed segment 0 on a rung nobody read (prod 2026-10-12).
func (h *hlsVODServe) prewarmSegment(mediaID string, durationSec float64, now time.Time) (int, bool) {
	seg, ok := h.startHints.peek(mediaID, now)
	if !ok {
		return 0, false
	}
	if durationSec > 0 && float64(seg)*apptranscoding.ManifestSegmentSeconds >= durationSec {
		return 0, true
	}
	return seg, true
}

// initSegment: where an init request ensures its rafale. Every rafale writes the
// same init, so: the fresh master's start, else the last segment served on this
// decision (a rung first asked mid-viewing, after a long pause), else 0.
func (h *hlsVODServe) initSegment(mediaID, fpHash string, durationSec float64, now time.Time) int {
	if seg, ok := h.prewarmSegment(mediaID, durationSec, now); ok {
		return seg
	}
	if seg, ok := h.livePositions.peekWithin(fpHash, now, livePositionTTL); ok {
		return seg
	}
	return 0
}
