package transcoding

// software_producer_gc.go holds SoftwareProducer's idle-sweep goroutine (moved
// out of software_producer.go for readability; pure move, no behaviour change).

import (
	"github.com/eveyproject/evey/internal/pkg/bgloop"
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
			bgloop.Tick("software.gc", p.sweepIdle)
			bgloop.Tick("software.killPaused", p.sweepKillPaused)
		case <-pauseTicker.C:
			bgloop.Tick("software.pause", p.sweepPause)
			bgloop.Tick("software.copyWindow", p.sweepCopyWindow)
		}
	}
}

// sweepIdle kills sessions whose lastAccess is older than InactivityTimeout AND
// frees their disk segments (the idle session won't be resumed; reclaim disk).
func (p *SoftwareProducer) sweepIdle() {
	cutoff := time.Now().Add(-p.opts.InactivityTimeout)
	// Critical section in a deferred closure: now that bgloop.Tick recovers
	// panics, a plain Unlock at the end of the block is not reached when the
	// sweep panics, and p.mu - the lock EnsureSegmentRange also takes - would
	// stay held forever. The server would survive, frozen, with no restart.
	toKill := func() []*swSession {
		p.mu.Lock()
		defer p.mu.Unlock()
		var toKill []*swSession
		for _, list := range p.sessions {
			for _, s := range list {
				idle := func() bool {
					s.mu.Lock()
					defer s.mu.Unlock()
					return s.lastAccess.Before(cutoff)
				}()
				if idle {
					toKill = append(toKill, s)
				}
			}
		}
		return toKill
	}()
	for _, s := range toKill {
		p.killSession(s, "inactivity")
	}
	// Reclaim disk ONLY for hashes that are now fully idle (no live sibling
	// remains). A hash with a surviving session must keep its segments - that
	// sibling (another tab/device) is still serving from them.
	affected := make(map[string]struct{}, len(toKill))
	for _, s := range toKill {
		affected[s.hash] = struct{}{}
	}
	for hash := range affected {
		empty := func() bool {
			p.mu.Lock()
			defer p.mu.Unlock()
			return len(p.sessions[hash]) == 0
		}()
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
		// Sessions AND disk are both reclaimed - drop the hash from the
		// Close-time purge set. Without this delete the set only ever grew
		// (one entry per distinct hash, for the process lifetime): a slow
		// unbounded leak over weeks of uptime. Re-checked under the lock - a
		// fresh session may have spawned for this hash since the emptiness
		// check, and that one still wants its Close-time purge.
		func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			if len(p.sessions[hash]) == 0 {
				delete(p.spawnedHashes, hash)
			}
		}()
	}
}
