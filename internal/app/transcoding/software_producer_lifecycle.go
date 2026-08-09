package transcoding

// software_producer_lifecycle.go holds SoftwareProducer's session teardown,
// eviction, and shutdown machinery (moved out of software_producer.go for
// readability; pure move, no behaviour change).

import (
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/pkg/metrics"
)

// RemoveSession kills EVERY session producing for the given media ID - across
// all hashes (different audio/sub/codec decisions) and all positions. This is
// the FORCEFUL path (admin kill-session): it tears sessions down regardless of
// whether another viewer is consuming them. The gentle playback-stop path must
// NOT call this unconditionally - the StopStream handler gates it on "no other
// active stream for this media" so closing one of two tabs never kills the
// other. No-op when none match.
func (p *SoftwareProducer) RemoveSession(mediaItemID string) {
	p.mu.Lock()
	var toKill []*swSession
	for _, list := range p.sessions {
		for _, s := range list {
			s.mu.Lock()
			match := s.mediaID == mediaItemID
			s.mu.Unlock()
			if match {
				toKill = append(toKill, s)
			}
		}
	}
	p.mu.Unlock()
	for _, s := range toKill {
		p.killSession(s, "remove_session")
	}
}

// RemoveHash kills every session encoding for ONE decision hash and reports
// how many it killed. The surgical counterpart of RemoveSession: a viewer who
// switched audio track (or subtitle burn-in, or audio offset) re-fingerprints
// the same viewing session, so the hash they left is dead weight holding a
// video-encode slot - but the OTHER hashes of the same media may still have a
// live viewer on them, and RemoveSession (media-wide) would take those down
// too. Callers must have established that nobody is on this hash anymore (see
// StreamHashRegistry). Disk segments are preserved exactly like every other
// kill path, so a viewer who switches back replays the cache instantly.
// No-op on an empty or unknown hash. Sessions fetched within reclaimMinIdle are
// spared - see that constant: the registry only sees token-carrying streams, so
// a JWT-authenticated client holds an encoder nothing else can vouch for.
func (p *SoftwareProducer) RemoveHash(hash, reason string) int {
	if hash == "" {
		return 0
	}
	cutoff := time.Now().Add(-reclaimMinIdle)
	p.mu.Lock()
	var toKill []*swSession
	for _, s := range p.sessions[hash] {
		s.mu.Lock()
		idle := s.lastAccess.Before(cutoff)
		s.mu.Unlock()
		if idle {
			toKill = append(toKill, s)
		}
	}
	p.mu.Unlock()
	for _, s := range toKill {
		p.killSession(s, reason)
	}
	return len(toKill)
}

// killSession stops ffmpeg, cancels its context, and releases the cache pending
// refcount. Idempotent. Does NOT delete disk segments (a rewind may want them);
// the idle GC and Close handle disk cleanup.
func (p *SoftwareProducer) killSession(s *swSession, reason string) {
	s.mu.Lock()
	if s.cancelCtx == nil {
		s.mu.Unlock()
		return
	}
	cancel := s.cancelCtx
	handle := s.handle
	hash := s.hash
	teardown := s.teardown
	s.cancelCtx = nil
	s.teardown = nil
	s.mu.Unlock()

	if handle != nil {
		if err := handle.Resume(); err != nil {
			p.logger.Debug("software: resume before stop failed (ignored)", slog.String("hash", hash), slog.Any("err", err))
		}
		if err := handle.Stop(); err != nil {
			p.logger.Warn("software: ffmpeg stop failed",
				slog.String("hash", hash), slog.String("reason", reason), slog.Any("err", err))
		}
	}
	cancel()
	if teardown != nil {
		teardown()
	}
	p.removeFromMap(hash, s)

	p.logger.Info("software: session killed", slog.String("hash", hash), slog.String("reason", reason))
	metrics.HLSVODSessionsKilled.WithLabelValues(reason).Inc()
	// Once-guarded: the reap goroutine also decrements on ffmpeg exit, so this
	// kill-path Dec must not double-count. Whichever runs first wins; the other
	// is a no-op.
	s.metricDecOnce.Do(func() { metrics.HLSVODSessions.Dec() })
	// Same exactly-once race against the reap goroutine (cancel() above wakes
	// it via sessionCtx.Done()) - releaseVideoSlot's Once ensures only the
	// first of the two frees the slot.
	p.releaseVideoSlot(s)
}

// removeFromMap drops a specific session from the hash's slice (a no-op if it
// was already spliced out by an eviction). Deletes the hash key when its last
// session is gone.
func (p *SoftwareProducer) removeFromMap(hash string, target *swSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := p.sessions[hash]
	for i, s := range list {
		if s == target {
			p.sessions[hash] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(p.sessions[hash]) == 0 {
		delete(p.sessions, hash)
	}
}

// evictLRUIfFullLocked enforces MaxSessionsPerHash by selecting the
// least-recently-touched sibling for eviction once the cap is exceeded. Run
// AFTER the new session has been appended, so the freshly-spawned one (at index
// len-1) is never the victim. Splices the victim out of the slice and returns
// it so the caller runs killSession outside the lock (killSession reacquires
// p.mu). Returns nil when under the cap. Must be called with p.mu held.
func (p *SoftwareProducer) evictLRUIfFullLocked(hash string) *swSession {
	list := p.sessions[hash]
	if len(list) <= p.opts.MaxSessionsPerHash {
		return nil
	}
	var victim *swSession
	victimIdx := -1
	var oldest time.Time
	now := time.Now()
	for i := range list[:len(list)-1] {
		s := list[i]
		s.mu.Lock()
		la := s.lastAccess
		newborn := p.newbornLocked(s, now)
		s.mu.Unlock()
		if newborn {
			// A session that has not yet delivered its first segment is what
			// an active Wait depends on: killing it turns overload into the
			// 2026-07-15 perpetual-spinner livelock (kill → waiter retries →
			// respawn → kills the next sibling → ...). It rides over the cap
			// for the grace window instead - SIGSTOPped by the pause machinery,
			// so it costs memory, not CPU.
			continue
		}
		if victim == nil || la.Before(oldest) {
			victim = s
			victimIdx = i
			oldest = la
		}
	}
	if victim == nil {
		return nil
	}
	p.sessions[hash] = append(list[:victimIdx], list[victimIdx+1:]...)
	return victim
}

// newbornLocked reports that s has not yet delivered its first segment and is
// still inside its eviction-shield window. Caller must hold s.mu.
func (p *SoftwareProducer) newbornLocked(s *swSession, now time.Time) bool {
	return s.frontier <= s.startSeg && now.Sub(s.spawnedAt) < p.opts.NewbornEvictionShield
}

// evictExcessVariantsLocked enforces MaxConcurrentVariantsPerHash: if the hash now
// has more DISTINCT live rungs than the cap, it drops the least-recently-used rungs
// (never keepVariant, the one just spawned) and returns every session of those
// rungs for the caller to kill outside the lock. A weak NAS cannot run a full
// decode per rung, so the number of distinct concurrently-encoding rungs for one
// film stays hard-bounded no matter how a client probes. Must hold p.mu.
func (p *SoftwareProducer) evictExcessVariantsLocked(hash string, keepVariant int) []*swSession {
	list := p.sessions[hash]
	newest := map[int]time.Time{} // most-recent access per distinct rung
	shielded := map[int]bool{}    // rung holds a newborn: see newbornLocked
	now := time.Now()
	for _, s := range list {
		s.mu.Lock()
		la, v := s.lastAccess, s.variantIdx
		if p.newbornLocked(s, now) {
			// A rung whose encoder has not yet delivered its first segment is
			// what an active Wait depends on - killing it is the 2026-07-15
			// spinner livelock. The rung rides over the cap (SIGSTOPped) for
			// the grace window instead.
			shielded[v] = true
		}
		s.mu.Unlock()
		if la.After(newest[v]) {
			newest[v] = la
		}
	}
	if len(newest) <= p.opts.MaxConcurrentVariantsPerHash {
		return nil
	}
	evict := map[int]struct{}{}
	for len(newest)-len(evict) > p.opts.MaxConcurrentVariantsPerHash {
		lru, have := -1, false
		var lruTime time.Time
		for v, t := range newest {
			if v == keepVariant || shielded[v] {
				continue
			}
			if _, gone := evict[v]; gone {
				continue
			}
			if !have || t.Before(lruTime) {
				lru, lruTime, have = v, t, true
			}
		}
		if !have {
			break
		}
		evict[lru] = struct{}{}
	}
	if len(evict) == 0 {
		return nil
	}
	var victims []*swSession
	remaining := make([]*swSession, 0, len(list))
	for _, s := range list {
		s.mu.Lock()
		v := s.variantIdx
		s.mu.Unlock()
		if _, gone := evict[v]; gone {
			victims = append(victims, s)
		} else {
			remaining = append(remaining, s)
		}
	}
	p.sessions[hash] = remaining
	return victims
}

// Close stops the GC, kills every live session, and purges every spawned hash's
// disk segments. Safe to call once.
func (p *SoftwareProducer) Close() {
	p.closeOnce.Do(p.doClose)
}

func (p *SoftwareProducer) doClose() {
	close(p.stopGC)
	<-p.gcDone

	// Cancel shutdownCtx first so in-flight PreWarm goroutines see it before
	// spawning, then drain them so cache cleanup isn't racing file writes.
	p.shutdownCancel()
	p.prewarmWG.Wait()

	p.mu.Lock()
	hashes := make([]string, 0, len(p.spawnedHashes))
	for h := range p.spawnedHashes {
		hashes = append(hashes, h)
	}
	live := p.sessions
	p.sessions = make(map[string][]*swSession)
	p.spawnedHashes = make(map[string]struct{})
	p.mu.Unlock()

	for _, hash := range hashes {
		for _, s := range live[hash] {
			p.killSession(s, "close")
		}
		if err := p.cache.CleanupHash(hash); err != nil {
			p.logger.Warn("software: close cleanup failed", slog.String("hash", hash), slog.Any("err", err))
		}
	}
}

// CleanupAll mirrors the orchestrator's shutdown alias so the wiring side can
// defer it without ambiguity. Same observable behaviour as Close.
func (p *SoftwareProducer) CleanupAll() { p.Close() }
