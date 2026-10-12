package transcoding

import (
	"sync"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// probe_signals.go - the runtime signal the read path records for the P3 probe
// worker WITHOUT ever running a probe itself. A cost-modelled rung whose
// produced segment ships at or above its advertised bitrate is evidence the
// model under-priced the title; the serve path increments a cheap in-memory
// counter, the worker consumes it in the background. The store is transient by
// design: a restart loses the counts and they re-accumulate from playback, so
// nothing durable depends on it (the durable triggers read persisted metrics).

// ProbeSignalStore counts per-title ceiling hits, bounded and concurrent.
type ProbeSignalStore struct {
	mu   sync.Mutex
	hits map[string]int
	// max bounds the map so a long uptime over a large library cannot grow it
	// without limit; the least-useful entries (below the trigger) are dropped
	// first when it is full.
	max int
}

// NewProbeSignalStore returns a store bounded to max distinct titles (<=0 → 4096).
func NewProbeSignalStore(max int) *ProbeSignalStore {
	if max <= 0 {
		max = 4096
	}
	return &ProbeSignalStore{hits: make(map[string]int), max: max}
}

// RecordCeilingHit adds one ceiling hit for a title. Called from the serve
// path; O(1), never blocks on anything but the map lock. Capped per title so a
// long playback cannot overflow the counter.
func (s *ProbeSignalStore) RecordCeilingHit(mediaID string) {
	if s == nil || mediaID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.hits[mediaID]; !ok && len(s.hits) >= s.max {
		s.evictBelowThresholdLocked()
		if len(s.hits) >= s.max {
			return // still full of hot candidates: drop this new one rather than a candidate
		}
	}
	if s.hits[mediaID] < ceilingHitCap {
		s.hits[mediaID]++
	}
}

// ceilingHitCap bounds one title's count: a handful past the trigger is enough
// to select it, and it stops one long stream from pinning the counter high.
const ceilingHitCap = transcoding.ProbeCeilingHitThreshold * 2

// Signals returns the ceiling-hit count for a title (0 when none).
func (s *ProbeSignalStore) Signals(mediaID string) transcoding.ProbeSignals {
	if s == nil {
		return transcoding.ProbeSignals{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return transcoding.ProbeSignals{CeilingHitSegments: s.hits[mediaID]}
}

// CeilingHitCandidates lists the titles whose ceiling-hit count reached the
// trigger, so the worker can consider them without scanning the library.
func (s *ProbeSignalStore) CeilingHitCandidates() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for id, n := range s.hits {
		if n >= transcoding.ProbeCeilingHitThreshold {
			out = append(out, id)
		}
	}
	return out
}

// Clear forgets a title's count - called once the worker has probed it, so a
// re-priced title starts accumulating fresh evidence against the new price.
func (s *ProbeSignalStore) Clear(mediaID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.hits, mediaID)
}

// evictBelowThresholdLocked drops entries under the trigger to make room; the
// caller holds the lock.
func (s *ProbeSignalStore) evictBelowThresholdLocked() {
	for id, n := range s.hits {
		if n < transcoding.ProbeCeilingHitThreshold {
			delete(s.hits, id)
		}
	}
}
