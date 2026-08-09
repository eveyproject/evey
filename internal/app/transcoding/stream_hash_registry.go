package transcoding

import (
	"sync"
	"time"
)

// StreamHashRegistry answers one question the decision hash alone cannot:
// when ONE viewer's player moves from one DecisionHash to another, which hash
// did it just abandon, and is anybody else still on it?
//
// The gap it closes: a track switch, a subtitle burn-in change or an audio
// offset re-fingerprints the SAME viewing session (AudioTrack and friends are
// Hash() dimensions), so the server sees a brand-new stream. The
// VideoSessionLimiter bills per hash, and the abandoned encoder keeps its
// video-encode slot until the idle/paused sweeps reap it minutes later - on a
// box calibrated at one concurrent encode that is minutes of "the server is
// full" caused by nobody but the viewer themself.
//
// The identity used is the STREAM TOKEN, not the media or the admission pin:
// a pin key (media|codec|decision) is shared by every viewer of a title, so
// reclaiming on it would tear down a second viewer's encoder. One token is one
// player instance, and it survives a track switch (the player rebuilds its URL
// around the same token), which is exactly the equivalence we need.
//
// ONE token holds exactly ONE hash, including mid-transition: a player still
// draining the old rendition has its old encoder killed on the first new-hash
// request. Deliberate, and it rests on the disk cache - killSession never
// deletes segments, so the drain reads what is already there. RemoveHash's
// reclaimMinIdle grace is the other half.
//
// Bounded on both axes: entries older than ttl are swept (a tab that vanished
// must not protect its hash forever) and the map never exceeds maxStreams
// (the coldest entry is evicted, releasing its hold). Both bounds fail SAFE
// in opposite directions on purpose - an over-eager sweep at worst reclaims an
// encoder whose disk segments survive (killSession never deletes them, so a
// returning viewer replays the cache while a fresh ffmpeg cold-starts), while
// a missed sweep merely leaves today's behaviour unchanged.
type StreamHashRegistry struct {
	mu      sync.Mutex
	streams map[string]*streamHashEntry
	// holders counts how many live streams sit on each hash. A hash is only
	// reclaimable once this reaches zero.
	holders    map[string]int
	ttl        time.Duration
	maxStreams int
	now        func() time.Time
}

type streamHashEntry struct {
	mediaID  string
	hash     string
	lastSeen time.Time
}

const (
	defaultStreamHashTTL     = 90 * time.Second
	defaultMaxTrackedStreams = 512
)

// NewStreamHashRegistry builds a registry. Non-positive ttl / maxStreams fall
// back to the defaults (90 s - comfortably longer than any player's segment
// cadence, so an actively-fetching stream is never swept - and 512 streams).
func NewStreamHashRegistry(ttl time.Duration, maxStreams int) *StreamHashRegistry {
	if ttl <= 0 {
		ttl = defaultStreamHashTTL
	}
	if maxStreams <= 0 {
		maxStreams = defaultMaxTrackedStreams
	}
	return &StreamHashRegistry{
		streams:    make(map[string]*streamHashEntry),
		holders:    make(map[string]int),
		ttl:        ttl,
		maxStreams: maxStreams,
		now:        time.Now,
	}
}

// Bind records that the stream identified by streamKey is now serving mediaID
// under fingerprint hash. It returns the hash the stream just LEFT, and true,
// ONLY when no other live stream is still on that hash - i.e. when its
// encoders are genuinely dead weight and the caller may reclaim them.
//
// Returns ("", false) on first sight, on a rebind of the same hash (the
// steady-state call, which just refreshes freshness), when another stream
// still holds the old hash, and for empty inputs (a share link or a
// hand-built test with no token must never collapse every anonymous viewer
// onto one entry).
func (r *StreamHashRegistry) Bind(streamKey, mediaID, hash string) (superseded string, ok bool) {
	if r == nil || streamKey == "" || hash == "" {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	r.sweepLocked(now)

	entry, known := r.streams[streamKey]
	if !known {
		r.evictIfFullLocked()
		r.streams[streamKey] = &streamHashEntry{mediaID: mediaID, hash: hash, lastSeen: now}
		r.holders[hash]++
		return "", false
	}

	entry.lastSeen = now
	if entry.hash == hash {
		entry.mediaID = mediaID
		return "", false
	}

	old := entry.hash
	entry.hash = hash
	entry.mediaID = mediaID
	r.holders[hash]++
	if r.releaseLocked(old) {
		return old, true
	}
	return "", false
}

// Forget drops a stream's entry and releases its hold - the explicit
// counterpart of the TTL sweep, for a caller that KNOWS a playback stopped.
// An unknown key is a no-op rather than a corrupting decrement.
func (r *StreamHashRegistry) Forget(streamKey string) {
	if r == nil || streamKey == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, known := r.streams[streamKey]
	if !known {
		return
	}
	delete(r.streams, streamKey)
	r.releaseLocked(entry.hash)
}

// Holders reports how many live streams currently sit on hash. Read-only, for
// tests and observability.
func (r *StreamHashRegistry) Holders(hash string) int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.holders[hash]
}

// Len reports how many streams are tracked. Read-only, for tests and the
// bounded-growth assertion.
func (r *StreamHashRegistry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.streams)
}

// releaseLocked drops one hold on hash and reports whether that was the LAST
// one. Must be called with r.mu held.
func (r *StreamHashRegistry) releaseLocked(hash string) bool {
	if hash == "" {
		return false
	}
	n := r.holders[hash]
	if n <= 1 {
		delete(r.holders, hash)
		return true
	}
	r.holders[hash] = n - 1
	return false
}

// sweepLocked drops entries no request has touched for ttl, releasing their
// holds. A player fetches a segment every few seconds, so only genuinely gone
// streams (closed tab, crashed client) ever expire. Must be called with r.mu
// held.
func (r *StreamHashRegistry) sweepLocked(now time.Time) {
	for key, e := range r.streams {
		if now.Sub(e.lastSeen) >= r.ttl {
			delete(r.streams, key)
			r.releaseLocked(e.hash)
		}
	}
}

// evictIfFullLocked bounds the map by dropping the coldest entry (and its
// hold) when a newcomer would push it past maxStreams. Must be called with
// r.mu held.
func (r *StreamHashRegistry) evictIfFullLocked() {
	if len(r.streams) < r.maxStreams {
		return
	}
	coldestKey, coldest := "", time.Time{}
	for key, e := range r.streams {
		if coldestKey == "" || e.lastSeen.Before(coldest) {
			coldestKey, coldest = key, e.lastSeen
		}
	}
	if coldestKey == "" {
		return
	}
	e := r.streams[coldestKey]
	delete(r.streams, coldestKey)
	r.releaseLocked(e.hash)
}
