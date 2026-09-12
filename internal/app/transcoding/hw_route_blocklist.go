package transcoding

import (
	"sync"
	"time"
)

// HWRouteBlocklist remembers the titles whose GPU route just failed, so the
// client's very next retry - and every other viewer of the same title - is
// served from the CPU instead of replaying the same broken session until the
// player gives up. It is the per-title half of the fallback ladder: session
// (the failed ffmpeg is gone), title (here), host (CalibrationService, only
// once a media-independent probe confirms the GPU itself is at fault).
//
// In memory on purpose: a restart is a fresh host, and the calibration's
// route probes decide what the GPU may take from then on. Entries expire
// after TTL so a transient cause (a busy GPU at boot) does not pin a title to
// the CPU for the rest of the process lifetime; the cap bounds a hostile burst
// of distinct titles at a few kB.
type HWRouteBlocklist struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	entries map[string]hwRouteBlock
	now     func() time.Time
}

type hwRouteBlock struct {
	reason string
	until  time.Time
}

// HWRouteBlockEntry is one blocked title, for the admin view.
type HWRouteBlockEntry struct {
	MediaID string    `json:"media_id"`
	Reason  string    `json:"reason"`
	Until   time.Time `json:"until"`
}

const (
	defaultHWRouteBlockTTL = 4 * time.Hour
	defaultHWRouteBlockMax = 4096
)

// NewHWRouteBlocklist builds a blocklist; non-positive ttl/max take the defaults.
func NewHWRouteBlocklist(ttl time.Duration, max int) *HWRouteBlocklist {
	if ttl <= 0 {
		ttl = defaultHWRouteBlockTTL
	}
	if max <= 0 {
		max = defaultHWRouteBlockMax
	}
	return &HWRouteBlocklist{ttl: ttl, max: max, entries: make(map[string]hwRouteBlock), now: time.Now}
}

// Block sends mediaID's GPU route to software for the TTL. Re-blocking a title
// refreshes its expiry and reason. Nil-safe.
func (b *HWRouteBlocklist) Block(mediaID, reason string) {
	if b == nil || mediaID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	b.evictLocked(now)
	if _, held := b.entries[mediaID]; !held && len(b.entries) >= b.max {
		// Full: drop the entry closest to expiry rather than refuse the new one
		// - the title that just failed is the one that must not replay.
		var oldest string
		var oldestUntil time.Time
		for id, e := range b.entries {
			if oldest == "" || e.until.Before(oldestUntil) {
				oldest, oldestUntil = id, e.until
			}
		}
		delete(b.entries, oldest)
	}
	b.entries[mediaID] = hwRouteBlock{reason: reason, until: now.Add(b.ttl)}
}

// Blocked reports whether mediaID's GPU route is currently sent to software.
// Nil-safe: an unwired blocklist blocks nothing.
func (b *HWRouteBlocklist) Blocked(mediaID string) bool {
	if b == nil || mediaID == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[mediaID]
	if !ok {
		return false
	}
	if !b.now().Before(e.until) {
		delete(b.entries, mediaID)
		return false
	}
	return true
}

// Unblock clears a title (a re-bench that proved new routes, an admin action).
func (b *HWRouteBlocklist) Unblock(mediaID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.entries, mediaID)
	b.mu.Unlock()
}

// Clear drops every entry - a fresh calibration re-proves the routes, so what
// the old proof blocked is re-tried on the new one.
func (b *HWRouteBlocklist) Clear() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.entries = make(map[string]hwRouteBlock)
	b.mu.Unlock()
}

// Entries lists the live blocks, expired ones pruned.
func (b *HWRouteBlocklist) Entries() []HWRouteBlockEntry {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evictLocked(b.now())
	out := make([]HWRouteBlockEntry, 0, len(b.entries))
	for id, e := range b.entries {
		out = append(out, HWRouteBlockEntry{MediaID: id, Reason: e.reason, Until: e.until})
	}
	return out
}

func (b *HWRouteBlocklist) evictLocked(now time.Time) {
	for id, e := range b.entries {
		if !now.Before(e.until) {
			delete(b.entries, id)
		}
	}
}
