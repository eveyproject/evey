package transcoding

// software_producer_gc.go holds SoftwareProducer's idle-sweep goroutine (moved
// out of software_producer.go for readability; pure move, no behaviour change).

import (
	"log/slog"
	"time"
)

// gcLoop sweeps idle sessions and suspends the encoder for clients that went
// quiet. Stopped via Close.
func (p *SoftwareProducer) gcLoop() {
	defer close(p.gcDone)
	gcTicker := time.NewTicker(p.opts.GCInterval)
	defer gcTicker.Stop()
	pauseTicker := time.NewTicker(p.opts.PauseIdleThreshold / 2)
	defer pauseTicker.Stop()
	for {
		select {
		case <-p.stopGC:
			return
		case <-gcTicker.C:
			p.sweepIdle()
			p.sweepKillPaused()
		case <-pauseTicker.C:
			p.sweepPause()
		}
	}
}

// sweepIdle kills sessions whose lastAccess is older than InactivityTimeout AND
// frees their disk segments (the idle session won't be resumed; reclaim disk).
func (p *SoftwareProducer) sweepIdle() {
	cutoff := time.Now().Add(-p.opts.InactivityTimeout)
	p.mu.Lock()
	var toKill []*swSession
	for _, list := range p.sessions {
		for _, s := range list {
			s.mu.Lock()
			idle := s.lastAccess.Before(cutoff)
			s.mu.Unlock()
			if idle {
				toKill = append(toKill, s)
			}
		}
	}
	p.mu.Unlock()
	for _, s := range toKill {
		p.killSession(s, "inactivity")
	}
	// Reclaim disk ONLY for hashes that are now fully idle (no live sibling
	// remains). A hash with a surviving session must keep its segments — that
	// sibling (another tab/device) is still serving from them.
	affected := make(map[string]struct{}, len(toKill))
	for _, s := range toKill {
		affected[s.hash] = struct{}{}
	}
	for hash := range affected {
		p.mu.Lock()
		empty := len(p.sessions[hash]) == 0
		p.mu.Unlock()
		if !empty {
			continue
		}
		// Eviction-safe reclaim: a fresh session can RegisterSession on this
		// hash between the emptiness check above and the disk cleanup. The
		// cache's pendingRefs guard (checked under the cache lock) closes that
		// window - an unconditional CleanupHash would delete the directory
		// under the newcomer and fail its ffmpeg rename into a 404/stall.
		evicted, err := p.cache.cleanupHashIfIdle(hash)
		if err != nil {
			p.logger.Warn("software: idle cleanup failed", slog.String("hash", hash), slog.Any("err", err))
			continue // keep it in spawnedHashes so Close retries the purge
		}
		if !evicted {
			continue // a just-registered session owns it now; Close still covers the purge
		}
		// Sessions AND disk are both reclaimed — drop the hash from the
		// Close-time purge set. Without this delete the set only ever grew
		// (one entry per distinct hash, for the process lifetime): a slow
		// unbounded leak over weeks of uptime. Re-checked under the lock — a
		// fresh session may have spawned for this hash since the emptiness
		// check, and that one still wants its Close-time purge.
		p.mu.Lock()
		if len(p.sessions[hash]) == 0 {
			delete(p.spawnedHashes, hash)
		}
		p.mu.Unlock()
	}
}
