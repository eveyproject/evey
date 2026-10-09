package transcoding

import (
	"errors"
	"sort"
	"time"
)

// ErrCacheFull means the transcode cache filesystem is below its free-space
// floor and on-demand eviction could not recover room, so a new segment must
// NOT be produced. The serve path turns it into a clean 503 + Retry-After
// instead of spawning an ffmpeg that dies on ENOSPC and is respawned on the
// next request (the storm of 2026-10-08: 69 ffmpeg starts for 11 requests).
// A segment already on disk is served regardless; this gates production only.
var ErrCacheFull = errors.New("transcode cache filesystem full")

// onDemandEvictCooldown bounds how often AdmitWrite pays for a full cache walk.
// Under a request storm it evicts at most once per window and otherwise does a
// single cheap statfs, so the guard never becomes the load it protects against.
const onDemandEvictCooldown = 2 * time.Second

// SetWriteFloor sets the free-space floor (bytes) AdmitWrite enforces. 0
// disables the guard (fail open). Match it to the eviction policy's MinFreeBytes
// so the on-demand pass and the background sweeper agree on "full".
func (c *SegmentCache) SetWriteFloor(floor int64) { c.writeFloor = floor }

func (c *SegmentCache) freeBytes() (int64, error) {
	if c.freeBytesFn != nil {
		return c.freeBytesFn()
	}
	return c.FreeBytes()
}

// AdmitWrite reports whether the cache filesystem has room to produce a new
// segment. Belt and braces:
//   - fail OPEN: floor 0, or any statfs error, never blocks playback;
//   - above the floor: one cheap statfs, no walk;
//   - below it: one rate-limited on-demand eviction of the oldest IDLE hashes,
//     then a recheck; still below -> ErrCacheFull.
//
// It never evicts a hash a live session serves (cleanupHashIfIdle), and the
// caller only consults it for a MISSING, non-copy segment, so a segment already
// on disk is always served even when the filesystem is full.
func (c *SegmentCache) AdmitWrite() error {
	floor := c.writeFloor
	if floor <= 0 {
		return nil
	}
	free, err := c.freeBytes()
	if err != nil {
		return nil //nolint:nilerr // fail open: our own guard must never break playback
	}
	if free >= floor {
		return nil
	}
	if c.tryOnDemandEvict(floor) {
		if free, err = c.freeBytes(); err != nil {
			return nil //nolint:nilerr // fail open: a statfs error after eviction must not block playback
		}
	}
	if free >= floor {
		return nil
	}
	return ErrCacheFull
}

// tryOnDemandEvict runs at most one eviction pass per cooldown. It removes the
// least-recently-used IDLE hashes (never one a live session serves) until free
// space clears the floor or nothing idle remains. Returns whether a pass ran.
func (c *SegmentCache) tryOnDemandEvict(floor int64) bool {
	c.evictMu.Lock()
	if !c.lastOnDemandEvict.IsZero() && time.Since(c.lastOnDemandEvict) < onDemandEvictCooldown {
		c.evictMu.Unlock()
		return false
	}
	c.lastOnDemandEvict = time.Now()
	c.evictMu.Unlock()

	usage, err := c.listHashUsage()
	if err != nil || len(usage) == 0 {
		return true // a pass ran (nothing evictable); the cooldown still applies
	}
	sort.Slice(usage, func(i, j int) bool { return usage[i].LastAccess.Before(usage[j].LastAccess) })
	for _, h := range usage {
		if free, ferr := c.freeBytes(); ferr == nil && free >= floor {
			break
		}
		_, _ = c.cleanupHashIfIdle(h.Hash) //nolint:errcheck // best-effort eviction; a hash a live session serves is skipped inside
	}
	return true
}
