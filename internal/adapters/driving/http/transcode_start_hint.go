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

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
)

// startHintTTL: the variant fetch follows the master within a round-trip; a
// minute keeps a hint from leaking into a later viewing.
const startHintTTL = time.Minute

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

func (s *startPositionHints) remember(mediaID string, seg int, now time.Time) {
	if mediaID == "" || seg <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]startHint)
	}
	for k, h := range s.m {
		if now.Sub(h.at) > startHintTTL {
			delete(s.m, k)
		}
	}
	s.m[mediaID] = startHint{seg: seg, at: now}
}

// peek is not a take: an ABR probe fetches two variant playlists for one start.
func (s *startPositionHints) peek(mediaID string, now time.Time) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.m[mediaID]
	if !ok || now.Sub(h.at) > startHintTTL {
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

// prewarmSegment: the remembered start when it lies inside the presentation, else 0.
func (h *hlsVODServe) prewarmSegment(mediaID string, durationSec float64, now time.Time) int {
	seg, ok := h.startHints.peek(mediaID, now)
	if !ok {
		return 0
	}
	if durationSec > 0 && float64(seg)*apptranscoding.ManifestSegmentSeconds >= durationSec {
		return 0
	}
	return seg
}
