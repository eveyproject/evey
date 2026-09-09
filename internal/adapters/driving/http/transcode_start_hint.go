package httpapi

// transcode_start_hint.go - where the player is about to start, so the
// variant-playlist pre-warm opens its rafale THERE instead of at segment 0.
//
// serveVariant pre-warms segment 0 on every variant-playlist fetch. It cannot
// know better on its own: the variant URL carries only the decision inputs, and
// no player names a position in a playlist request. A resume or an audio-track
// switch (the web player restarts the transcode at the current position) then
// opened a GPU encoder at 0 that the player never read, while its real
// position waited for one of its own. Prod, 2026-09-09: switch at segment 23,
// pre-warm at 0, spinner for over a minute.
//
// The master request DOES carry the position: the web player has always sent
// ?startSec= (documented on its side as "the server ignores it"). It is
// remembered here per media for the seconds between the master and the variant
// fetch that follows, and only ever used as a pre-warm hint - a wrong or stale
// one costs one speculative rafale, exactly what happened on every resume
// before, never a wrong segment.

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
)

// startHintTTL is how long a remembered position stays valid. hls.js fetches
// the variant playlist within a round-trip of the master; a minute covers a
// slow client several times over without letting a hint from an earlier
// viewing leak into the next.
const startHintTTL = time.Minute

type startHint struct {
	seg int
	at  time.Time
}

// startPositionHints remembers, per media, the segment a master request said
// playback starts at. The zero value is ready to use.
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
	// Opportunistic sweep: the map only ever holds films whose master was
	// fetched in the last minute, so this stays tiny.
	for k, h := range s.m {
		if now.Sub(h.at) > startHintTTL {
			delete(s.m, k)
		}
	}
	s.m[mediaID] = startHint{seg: seg, at: now}
}

// peek returns the remembered start segment for mediaID, if one is fresh. Not
// consumed: hls.js may fetch two variant playlists for one start (an ABR
// probe), and both should warm at the same place.
func (s *startPositionHints) peek(mediaID string, now time.Time) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.m[mediaID]
	if !ok || now.Sub(h.at) > startHintTTL {
		return 0, false
	}
	return h.seg, true
}

// startSegmentHint reads the player's ?startSec= off a master request and turns
// it into a segment index. Absent, zero, negative or unparsable = no hint.
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

// prewarmSegment is where serveVariant should open its rafale: the remembered
// start position when there is one inside the presentation, else 0. A hint
// past the end is a stale or bogus value and falls back to 0 rather than
// spawning an encoder at a position that does not exist.
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
