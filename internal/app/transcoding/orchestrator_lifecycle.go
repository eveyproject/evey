package transcoding

// orchestrator_lifecycle.go holds SegmentOrchestrator's session teardown,
// eviction, GC, and shutdown machinery (moved out of segment_orchestrator.go
// for readability; pure move, no behaviour change).

import (
	"github.com/evey-project/evey/internal/pkg/bgloop"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/pkg/metrics"
)

// Close stops the inactivity GC, kills all live sessions, and purges their
// disk segments. cancel-on-seek and inactivity-kill intentionally leave the
// hash on disk so future requests can hit it warm; Close is the explicit
// shutdown path where we DO want the cache freed (the orchestrator is going
// away - nothing will hit those segments again). Safe to call once.
func (o *SegmentOrchestrator) Close() {
	o.closeOnce.Do(o.doClose)
}

func (o *SegmentOrchestrator) doClose() {
	close(o.stopGC)
	<-o.gcDone
	if o.opts.ThrottleEnabled {
		close(o.stopThrottle)
	}
	<-o.throttleDone

	// Cancel shutdownCtx FIRST so in-flight PreWarm goroutines see the
	// cancellation before they try to spawn new sessions, then drain them
	// so cache cleanup below isn't racing with file writes.
	o.shutdownCancel()
	o.prewarmWG.Wait()

	// Snapshot every hash this orchestrator has ever spawned a session
	// for. Iterate it (not the live `sessions` map) so a sweepIdle that
	// deleted the session struct mid-shutdown still gets its disk cache
	// cleaned. killSession is idempotent and skips already-cleared
	// entries, so it's safe to call on a hash whose session is already
	// gone.
	o.mu.Lock()
	hashes := make([]string, 0, len(o.spawnedHashes))
	for h := range o.spawnedHashes {
		hashes = append(hashes, h)
	}
	liveSessions := o.sessions
	o.sessions = make(map[string][]*orchestratorSession)
	o.spawnedHashes = make(map[string]struct{})
	o.mu.Unlock()

	for _, hash := range hashes {
		for _, s := range liveSessions[hash] {
			o.killSession(s, "close")
		}
		if err := o.cache.CleanupHash(hash); err != nil {
			o.logger.Warn("orchestrator: close cleanup failed",
				slog.String("hash", hash),
				slog.Any("err", err))
		}
	}
}

// evictLRUIfFullLocked enforces MaxRafalesPerHash by killing the
// least-recently-touched rafale when the cap is reached. Run AFTER
// the new session has been appended to o.sessions[hash], so the
// freshly-spawned one is never the eviction candidate. Must be called
// with o.mu held.
//
// Returns the evicted session (or nil) so the caller can run killSession
// outside the lock - killSession itself reacquires o.mu.
func (o *SegmentOrchestrator) evictLRUIfFullLocked(hash string) *orchestratorSession {
	list := o.sessions[hash]
	if len(list) <= o.opts.MaxRafalesPerHash {
		return nil
	}
	// Find the LRU among the OLDER entries (skip the newly-appended one
	// at index len-1 - it has the freshest lastAccess by construction).
	var victim *orchestratorSession
	var victimIdx int
	var oldest time.Time
	for i := range list[:len(list)-1] {
		s := list[i]
		s.mu.Lock()
		la := s.lastAccess
		s.mu.Unlock()
		if victim == nil || la.Before(oldest) {
			victim = s
			victimIdx = i
			oldest = la
		}
	}
	if victim == nil {
		return nil
	}
	// Splice the victim out of the list - killSession does the rest
	// (close ffmpeg, teardown cache pending channel) without holding
	// o.mu so we don't deadlock when it reacquires the lock at the
	// end of its body.
	o.sessions[hash] = append(list[:victimIdx], list[victimIdx+1:]...)
	return victim
}

// killSession stops ffmpeg, releases the cancel context, and cleans up cache
// state. Idempotent: a second call on the same session is a no-op.
func (o *SegmentOrchestrator) killSession(s *orchestratorSession, reason string) {
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
	s.killed = true // before Stop(): the reap must never read this exit as natural
	// Null out teardown so the reap goroutine doesn't try to fire it
	// after we already did. The released bool inside the closure
	// remains the canonical idempotency guard at the cache layer, but
	// nulling here avoids spurious "released" returns.
	s.teardown = nil
	s.mu.Unlock()

	if handle != nil {
		// A throttled session is in SIGSTOP and will ignore ctx cancellation;
		// the Stop path Resume()s defensively too, but calling it here means
		// the session.paused flag and metrics stay coherent for the throttle
		// loop while Stop runs.
		if err := handle.Resume(); err != nil {
			o.logger.Warn("orchestrator: ffmpeg resume before stop failed",
				slog.String("hash", hash), slog.Any("err", err))
		}
		if err := handle.Stop(); err != nil {
			o.logger.Warn("orchestrator: ffmpeg stop failed",
				slog.String("hash", hash),
				slog.String("reason", reason),
				slog.Any("err", err))
		}
	}
	cancel()
	if teardown != nil {
		teardown()
	}

	o.removeSessionFromList(hash, s)

	o.logger.Info("orchestrator: session killed",
		slog.String("hash", hash),
		slog.String("reason", reason))

	metrics.HLSVODSessionsKilled.WithLabelValues(reason).Inc()
	// Exactly-once with the reap goroutine: a watchdog kill cancels sessionCtx,
	// which also wakes the reap - only the first of the two decrements the gauge.
	s.metricDecOnce.Do(func() { metrics.HLSVODSessions.Dec() })
	// Same exactly-once race against the reap goroutine (cancel() above wakes
	// it via sessionCtx.Done()) - releaseVideoSlot's Once ensures only the
	// first of the two frees the slot.
	o.releaseVideoSlot(s)
}

// removeSessionFromList drops `target` from o.sessions[hash], leaving
// the surviving rafales in place. Deletes the map entry when the
// slice goes empty so future len() reads see "no live rafale". Used
// by the reap goroutine (ffmpeg exited on its own) and killSession
// (explicit teardown).
func (o *SegmentOrchestrator) removeSessionFromList(hash string, target *orchestratorSession) {
	o.mu.Lock()
	defer o.mu.Unlock()
	list := o.sessions[hash]
	for i, s := range list {
		if s == target {
			o.sessions[hash] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(o.sessions[hash]) == 0 {
		delete(o.sessions, hash)
	}
}

// gcLoop runs in the background and kills sessions whose lastAccess is older
// than InactivityTimeout. Stopped via Close.
func (o *SegmentOrchestrator) gcLoop() {
	defer close(o.gcDone)
	ticker := time.NewTicker(o.opts.GCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-o.stopGC:
			return
		case <-ticker.C:
			bgloop.Tick("orchestrator.gc", o.sweepIdle)
		}
	}
}

func (o *SegmentOrchestrator) sweepIdle() {
	now := time.Now()
	cutoff := now.Add(-o.opts.InactivityTimeout)
	// A rafale no client has ever read is a speculative pre-warm whose bet did
	// not pay off; it gets the short fuse instead (see UnreadTimeout).
	unreadCutoff := now.Add(-o.opts.UnreadTimeout)
	// defer rather than a trailing Unlock: bgloop.Tick recovers panics, so
	// without it a panicking sweep would leave o.mu held forever and no hardware
	// session could start or stop again.
	type doomed struct {
		s      *orchestratorSession
		reason string
	}
	toKill := func() []doomed {
		o.mu.Lock()
		defer o.mu.Unlock()
		var toKill []doomed
		for _, list := range o.sessions {
			for _, s := range list {
				reason := func() string {
					s.mu.Lock()
					defer s.mu.Unlock()
					if !s.clientRead && s.lastAccess.Before(unreadCutoff) {
						return "unread_prewarm"
					}
					if s.lastAccess.Before(cutoff) {
						return "inactivity"
					}
					return ""
				}()
				if reason != "" {
					toKill = append(toKill, doomed{s: s, reason: reason})
				}
			}
		}
		return toKill
	}()
	for _, d := range toKill {
		o.killSession(d.s, d.reason)
	}
}

// CleanupAll mirrors the legacy HLSManager.CleanupAll - deferred from main
// at shutdown. Same observable behaviour as Close (kill every ffmpeg + free
// every hash on disk) but kept as a distinct method so the wiring side can
// `defer` it without ambiguity.
func (o *SegmentOrchestrator) CleanupAll() { o.Close() }
