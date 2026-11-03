package logger

import (
	"sync"
	"time"
)

// Sampler lets one line through per key and per window and counts the rest,
// so an outage that fails every call (a provider down for an hour, one
// warning per person and per translation) costs the log one line a minute
// per message, with the number of lines held back on it.
type Sampler struct {
	window time.Duration
	now    func() time.Time
	mu     sync.Mutex
	keys   map[string]*sampleEntry
}

type sampleEntry struct {
	until      time.Time
	suppressed int
}

// NewSampler builds a sampler that lets one line per key through per window.
func NewSampler(window time.Duration) *Sampler {
	return NewSamplerForTest(window, time.Now)
}

// NewSamplerForTest is NewSampler with an injected clock.
func NewSamplerForTest(window time.Duration, now func() time.Time) *Sampler {
	return &Sampler{window: window, now: now, keys: make(map[string]*sampleEntry)}
}

// Allow says whether this key's line is written now and, when it is, how many
// lines for the key were held back since the last one written.
func (s *Sampler) Allow(key string) (allowed bool, suppressed int) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.keys[key]
	if ok && now.Before(e.until) {
		e.suppressed++
		return false, 0
	}
	held := 0
	if ok {
		held = e.suppressed
	}
	s.keys[key] = &sampleEntry{until: now.Add(s.window)}
	return true, held
}
