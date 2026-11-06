package httpapi

import (
	"sync"
	"time"
)

// firstSegmentTTL bounds how long a master waits for its first media segment
// to count as a startup: past the longest producer wait budget plus the
// variant and init round-trips, the player has given up or the playback took
// a path that serves its segments elsewhere.
const firstSegmentTTL = 2 * time.Minute

// firstSegmentClock times a playback's startup: armed by the master playlist,
// taken by the first media segment served under the same viewer key. A take
// consumes the entry, so only the first segment of a playback is measured.
// The zero value is ready to use.
type firstSegmentClock struct {
	mu        sync.Mutex
	m         map[string]time.Time
	lastSweep time.Time
}

func (c *firstSegmentClock) arm(key string, now time.Time) {
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]time.Time)
	}
	// Masters with no segment behind them (another rung served the playback,
	// the viewer left) would otherwise stay forever: walk them out, at most
	// once per interval.
	if now.Sub(c.lastSweep) >= positionHintSweepEvery {
		c.lastSweep = now
		for k, at := range c.m {
			if now.Sub(at) > firstSegmentTTL {
				delete(c.m, k)
			}
		}
	}
	c.m[key] = now
}

func (c *firstSegmentClock) take(key string, now time.Time) (time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.m[key]
	if !ok {
		return 0, false
	}
	delete(c.m, key)
	if now.Sub(at) > firstSegmentTTL {
		return 0, false
	}
	return now.Sub(at), true
}
