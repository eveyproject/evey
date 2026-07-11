package transcoding

// software_producer_pacing.go holds SoftwareProducer's pause/resume/SIGSTOP-hint
// machinery (moved out of software_producer.go for readability; pure move, no
// behaviour change).

import (
	"log/slog"
	"time"
)

// NotifyClientHint resumes a paused encoder when the client reports starvation,
// and bumps lastAccess so the GC keeps the session warm. It NEVER changes which
// variant is served (there is only one rung anyway). The single paced encoder
// has no burst-ahead window to stretch, so a fat-buffer hint is a no-op beyond
// keeping the session alive.
func (p *SoftwareProducer) NotifyClientHint(mediaItemID string, bufferLengthMs int, starved bool, segmentIdx int) {
	_ = bufferLengthMs
	p.mu.Lock()
	var matched []*swSession
	for _, list := range p.sessions {
		for _, s := range list {
			s.mu.Lock()
			mid := s.mediaID
			s.mu.Unlock()
			if mid == mediaItemID {
				matched = append(matched, s)
			}
		}
	}
	p.mu.Unlock()
	// Resume ONLY the closest-behind session — the one whose window
	// [startSeg, ∞) actually serves segmentIdx (same selection rule as
	// findCoveringSessionLocked, applied across the mediaID's hashes).
	// Resuming every paused sibling let an abandoned unpaced copy session
	// race back toward EOF and REWRITE the active session's cache files —
	// the post-seek starvation hint (empty buffer → CMCD bs=1) was the
	// trigger of exactly that rewrite storm. lastAccess is still bumped on
	// all matches so the GC keeps consumed-from-disk sessions warm.
	var covering *swSession
	bestStart := -1
	for _, s := range matched {
		s.mu.Lock()
		s.lastAccess = time.Now()
		start := s.startSeg
		s.mu.Unlock()
		if start <= segmentIdx && start > bestStart {
			bestStart, covering = start, s
		}
	}
	if starved && covering != nil {
		p.resumeIfPaused(covering)
	}
}

// resumeIfPaused SIGCONTs the encoder if the idle-pause loop had suspended it.
// Idempotent on a running process; only fires on the paused→active edge.
func (p *SoftwareProducer) resumeIfPaused(s *swSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.paused || s.handle == nil {
		return
	}
	// Resume the process while still holding s.mu so a concurrent sweepPause /
	// pauseEach cannot interleave its SIGSTOP between this SIGCONT and the flag
	// flip and leave the encoder stopped while s.paused reads false — a
	// stuck-paused session no later resume would ever wake. Flip the flag only
	// on success so a failed Resume keeps s.paused=true for the next retry.
	if err := s.handle.Resume(); err != nil {
		p.logger.Warn("software: resume ffmpeg failed", slog.String("hash", s.hash), slog.Any("err", err))
		return
	}
	s.paused = false
	s.pausedSince = time.Time{}
}

// pauseEach SIGSTOPs each of the given sessions (skipping already-paused or
// handle-less ones), reverting the flag on a failed Pause. Shared by the
// seek/level-switch CPU-handoff paths. Pause, not kill: a session a viewer still
// consumes resumes on its next segment request (resumeIfPaused).
func (p *SoftwareProducer) pauseEach(sessions []*swSession) {
	for _, s := range sessions {
		s.mu.Lock()
		if s.paused || s.handle == nil {
			s.mu.Unlock()
			continue
		}
		// SIGSTOP under s.mu so the pause and the s.paused flag are atomic w.r.t.
		// a concurrent resumeIfPaused (see resumeIfPaused). Flag set only on a
		// successful Pause.
		if err := s.handle.Pause(); err != nil {
			s.mu.Unlock()
			p.logger.Debug("software: pause failed", slog.String("hash", s.hash), slog.Any("err", err))
			continue
		}
		s.paused = true
		s.pausedSince = time.Now()
		s.mu.Unlock()
	}
}

// pauseSiblingsBehind SIGSTOPs the SAME-rung encoders for hash whose start
// position is strictly BEHIND keep (a forward seek leaves them behind). It does
// NOT touch siblings AHEAD of keep (a deeper viewer / second tab still watching
// further in) nor OTHER rungs (that is pauseOtherVariants' job). On a box that
// fits one heavy software transcode, the just-seeked-to position takes the CPU
// immediately rather than fighting the abandoned encoder for PauseIdleThreshold
// seconds — the "small buffering after seeking" symptom.
func (p *SoftwareProducer) pauseSiblingsBehind(hash string, keep *swSession) {
	keep.mu.Lock()
	keepStart := keep.startSeg
	keepVariant := keep.variantIdx
	keep.mu.Unlock()

	p.mu.Lock()
	var behind []*swSession
	for _, s := range p.sessions[hash] {
		if s == keep {
			continue
		}
		s.mu.Lock()
		isBehind := s.variantIdx == keepVariant && s.startSeg < keepStart
		s.mu.Unlock()
		if isBehind {
			behind = append(behind, s)
		}
	}
	p.mu.Unlock()
	p.pauseEach(behind)
}

// pauseOtherVariants SIGSTOPs every live encoder for hash producing a DIFFERENT
// rung than keep. A fresh rung starting means the viewer switched levels (an AUTO
// degrade or a MANUAL pick), so the old rung is abandoned: suspend it AT ONCE
// rather than let it run a second full 4K decode against the new rung for up to
// PauseIdleThreshold seconds (the level-switch famine window the thrash review
// flagged). A rung a genuine co-viewer still consumes resumes on its next
// request. Pause, not kill — the distinct-variant cap reclaims the slot.
func (p *SoftwareProducer) pauseOtherVariants(hash string, keep *swSession) {
	keep.mu.Lock()
	keepVariant := keep.variantIdx
	keep.mu.Unlock()

	p.mu.Lock()
	var others []*swSession
	for _, s := range p.sessions[hash] {
		if s == keep {
			continue
		}
		s.mu.Lock()
		diff := s.variantIdx != keepVariant
		s.mu.Unlock()
		if diff {
			others = append(others, s)
		}
	}
	p.mu.Unlock()
	p.pauseEach(others)
}

// sweepPause SIGSTOPs the encoder for any session whose client has gone quiet
// past PauseIdleThreshold. A segment fetch (touch / EnsureSegmentRange) or a
// CMCD starvation hint resumes it. Saves CPU on a paused player without
// touching the (no-eviction) playlist.
func (p *SoftwareProducer) sweepPause() {
	cutoff := time.Now().Add(-p.opts.PauseIdleThreshold)
	p.mu.Lock()
	snapshot := make([]*swSession, 0, len(p.sessions))
	for _, list := range p.sessions {
		snapshot = append(snapshot, list...)
	}
	p.mu.Unlock()
	for _, s := range snapshot {
		s.mu.Lock()
		if s.paused || s.handle == nil || !s.lastAccess.Before(cutoff) {
			s.mu.Unlock()
			continue
		}
		hash := s.hash
		// SIGSTOP under s.mu so the pause and the s.paused flag are atomic w.r.t.
		// a concurrent resumeIfPaused (see resumeIfPaused).
		if err := s.handle.Pause(); err != nil {
			s.mu.Unlock()
			p.logger.Debug("software: pause ffmpeg failed", slog.String("hash", hash), slog.Any("err", err))
			continue
		}
		s.paused = true
		s.pausedSince = time.Now()
		s.mu.Unlock()
		p.logger.Debug("software: ffmpeg suspended — client idle", slog.String("hash", hash))
	}
}

// hasResumeCushion reports whether the segment cache already holds at least
// opts.ResumeCushionSegs CONTIGUOUS segments beyond frontier - the buffer that
// hides a killed session's cold-start ffmpeg from the player. Contiguous from
// frontier+1: a gap would mean the player hits an uncached segment before the
// fresh `-ss` encoder lands, which is exactly the latency this guard exists to
// prevent.
func (p *SoftwareProducer) hasResumeCushion(hash string, variantIdx, frontier int, useFMP4 bool) bool {
	for i := 1; i <= p.opts.ResumeCushionSegs; i++ {
		k := SegmentKey{DecisionHash: hash, VariantIdx: variantIdx, SegmentIdx: frontier + i}
		if !p.cache.IsReady(k, useFMP4) {
			return false
		}
	}
	return true
}

// sweepKillPaused reclaims the ffmpeg RSS (~1 GB) of sessions that have sat
// SIGSTOPped past KillPausedAfter - a memory-constrained box cannot afford to
// keep paying for a suspended encoder's resident memory just because a viewer
// might resume. It does NOT touch disk: killSession never deletes segments,
// and this sweep deliberately never calls CleanupHash, so a resumed player
// replays the cache instantly while a fresh `-ss` ffmpeg cold-starts behind
// it - the sweepIdle GC and the disk LRU evictor reclaim the segments later,
// on their own unrelated schedule.
//
// The anti-latency guarantee: a paused session is only killed once
// hasResumeCushion confirms the cache already holds ResumeCushionSegs
// segments beyond its frontier (enough to mask the fresh encoder's cold
// start). Below the cushion the session is SIGCONTed instead - it is paced
// fast (1.5x transcode / 8x copy) so it builds the missing cushion quickly -
// and lastAccess is bumped so the very next idle-pause tick doesn't
// immediately re-suspend it before the cushion has a chance to land; a later
// sweep revisits it once actually paused again.
//
// No-op when KillPausedAfter <= 0 (the default) - current behaviour is
// unchanged: a paused session then only ever dies via sweepIdle.
func (p *SoftwareProducer) sweepKillPaused() {
	if p.opts.KillPausedAfter <= 0 {
		return
	}
	cutoff := time.Now().Add(-p.opts.KillPausedAfter)

	p.mu.Lock()
	var toKill, toBuild []*swSession
	for _, list := range p.sessions {
		for _, s := range list {
			s.mu.Lock()
			eligible := s.paused && !s.pausedSince.IsZero() && s.pausedSince.Before(cutoff)
			hash, variantIdx, frontier, useFMP4 := s.hash, s.variantIdx, s.frontier, s.useFMP4
			s.mu.Unlock()
			if !eligible {
				continue
			}
			if p.hasResumeCushion(hash, variantIdx, frontier, useFMP4) {
				toKill = append(toKill, s)
			} else {
				toBuild = append(toBuild, s)
			}
		}
	}
	p.mu.Unlock()

	for _, s := range toKill {
		p.logger.Info("software: killing long-paused session, disk cache preserved for instant resume",
			slog.String("hash", s.hash), slog.Duration("paused_after", p.opts.KillPausedAfter),
			slog.Int("cushion_segs", p.opts.ResumeCushionSegs))
		p.killSession(s, "paused_ram_reclaim")
	}
	for _, s := range toBuild {
		s.mu.Lock()
		s.lastAccess = time.Now()
		s.mu.Unlock()
		p.resumeIfPaused(s)
	}
}
