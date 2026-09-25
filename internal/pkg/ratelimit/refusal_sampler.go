package ratelimit

import (
	"sync"
	"time"
)

// refusalSampleWindow is how long one (client, path) pair stays quiet after a
// logged refusal; the refusals in between are counted onto the next line.
const refusalSampleWindow = time.Minute

// refusalSamplerMaxKeys bounds the sampler's table. Past it the table is
// dropped whole: a client rotating addresses then gets its lines logged
// again, which is the pre-sampler behaviour, never a lost refusal.
const refusalSamplerMaxKeys = 10_000

// sampler keeps, per key, when the last line went out and how many refusals
// were swallowed since. The refusal itself is never affected - only its line.
type sampler struct {
	mu   sync.Mutex
	last map[string]*sampleEntry
}

type sampleEntry struct {
	loggedAt   time.Time
	suppressed int64
}

var refusalSampler = &sampler{last: make(map[string]*sampleEntry)}

// admit reports whether a refusal of key at now gets its log line, and how
// many refusals of that key were suppressed since the previous line.
func (s *sampler) admit(key string, now time.Time) (suppressed int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, seen := s.last[key]
	if seen && now.Sub(e.loggedAt) < refusalSampleWindow {
		e.suppressed++
		return 0, false
	}
	if !seen {
		if len(s.last) >= refusalSamplerMaxKeys {
			s.last = make(map[string]*sampleEntry)
		}
		e = &sampleEntry{}
		s.last[key] = e
	}
	suppressed = e.suppressed
	e.loggedAt = now
	e.suppressed = 0
	return suppressed, true
}

// resetForTest empties the sampler (SetLogger, i.e. a new sink).
func (s *sampler) resetForTest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = make(map[string]*sampleEntry)
}
