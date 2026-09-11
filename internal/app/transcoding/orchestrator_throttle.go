package transcoding

// orchestrator_throttle.go holds SegmentOrchestrator's pause-when-ahead
// throttle loop and CMCD hint handling (moved out of segment_orchestrator.go
// for readability; pure move, no behaviour change).

import (
	"github.com/evey-project/evey/internal/pkg/bgloop"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/pkg/metrics"
)

// throttleLoop applies pause-when-ahead to every live session on a ticker.
// Pause-when-ahead pattern (standard in mature transcode pipelines): when
// ffmpeg has produced ThrottlePauseAhead segments more than the client's
// high-water, SIGSTOP it - we burn 0 CPU until the client catches up.
// Hysteresis (ResumeBelow strictly less than PauseAhead) prevents flapping.
// The loop is a no-op when ThrottleEnabled is false (the goroutine isn't
// even started in that case).
func (o *SegmentOrchestrator) throttleLoop() {
	defer close(o.throttleDone)
	ticker := time.NewTicker(o.opts.ThrottleTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-o.stopThrottle:
			return
		case <-ticker.C:
			bgloop.Tick("orchestrator.throttle", o.evaluateThrottle)
		}
	}
}

// evaluateThrottle runs one tick of the pause-when-ahead policy. For each
// live session, ListReady gives the latest produced segment idx; the buffer
// (produced - client) decides pause vs resume.
func (o *SegmentOrchestrator) evaluateThrottle() {
	snapshot := func() []*orchestratorSession {
		o.mu.Lock()
		defer o.mu.Unlock()
		var out []*orchestratorSession
		for _, list := range o.sessions {
			out = append(out, list...)
		}
		return out
	}()

	for _, s := range snapshot {
		s.mu.Lock()
		hash := s.hash
		client := s.clientHighWater
		paused := s.paused
		handle := s.handle
		useFMP4 := s.useFMP4
		variantIdx := s.variantIdx
		lastProduced := s.producedHighWater
		startSeg := s.startSeg
		endSeg := s.endSeg
		idle := time.Since(s.lastAccess)
		s.mu.Unlock()

		if handle == nil {
			continue
		}

		// Track this session's own rung: in SingleVariant mode a rung>0 session
		// writes stream_<variantIdx>_*, so listing variant 0 would always find
		// nothing and never throttle it (CPU/disk waste). In the default
		// multi-variant mode stream_0 tracks the shared session, so variantIdx==0.
		//
		// Resuming from the previous tick's high-water keeps this O(segments
		// produced since last tick) - normally 0-2 stats - instead of the
		// os.ReadDir + full-directory parse ListReady used to pay every second,
		// per session, on the same disk the player is streaming from.
		produced := o.cache.HighestReadyFrom(hash, variantIdx, useFMP4, lastProduced+1, endSeg)
		if produced < startSeg {
			// Nothing has landed yet for this rafale: no buffer to reason about.
			continue
		}
		if produced != lastProduced {
			s.mu.Lock()
			s.producedHighWater = produced
			s.mu.Unlock()
		}
		buffer := produced - client

		// An unread rafale that is ahead of its last reader has lost its viewer
		// (seek elsewhere, cache hits): pause it now rather than at PauseAhead.
		abandoned := buffer >= 1 && idle >= o.opts.IdlePauseAfter
		switch {
		case !paused && (buffer >= o.opts.ThrottlePauseAhead || abandoned):
			if err := handle.Pause(); err != nil {
				o.logger.Warn("orchestrator: throttle pause failed",
					slog.String("hash", hash), slog.Any("err", err))
				continue
			}
			s.mu.Lock()
			s.paused = true
			s.mu.Unlock()
			o.logger.Debug("orchestrator: throttle paused",
				slog.String("hash", hash),
				slog.Int("client_idx", client),
				slog.Int("produced_idx", produced),
				slog.Int("buffer", buffer),
				slog.Bool("abandoned", abandoned))
			metrics.HLSVODThrottlePause.Inc()
		case paused && buffer <= o.opts.ThrottleResumeBelow:
			if err := handle.Resume(); err != nil {
				o.logger.Warn("orchestrator: throttle resume failed",
					slog.String("hash", hash), slog.Any("err", err))
				continue
			}
			s.mu.Lock()
			s.paused = false
			s.mu.Unlock()
			o.logger.Debug("orchestrator: throttle resumed",
				slog.String("hash", hash),
				slog.Int("client_idx", client),
				slog.Int("produced_idx", produced),
				slog.Int("buffer", buffer))
			metrics.HLSVODThrottleResume.Inc()
		}
	}
}

// NotifyClientHint feeds CMCD telemetry from the HTTP layer back into the
// orchestrator so the throttle / prefetch policy can react to what the
// client is actually experiencing - buffer length, imminent starvation,
// real cursor - instead of the coarse "last requested segment" proxy.
//
// CRITICAL constraint: this method NEVER changes which variant the client
// downloads. The HLS protocol gives the player full control over rendition
// selection (the segment URL carries the variant index). All we can do
// here is adjust the encoder's pace and lookahead, and wake up a paused
// ffmpeg - all transparent to the client and to the user's manual quality
// lock.
//
// Effects per session whose mediaID matches:
//
//   - clientHighWater bumped forward when the CMCD-derived cursor is
//     ahead of what touchSession had previously seen. We only ever bump
//     forward - a stale CMCD packet must not pull the throttle back.
//
//   - On bs=1 (buffer starved), SIGCONT a paused session immediately
//     instead of waiting up to ThrottleTickInterval. The client has
//     already stopped playback, so resume latency directly hits rebuffer.
//
//   - On large bl (client comfortably ahead), stretch endSeg so the next
//     EnsureSegmentRange call doesn't have to extend the rafale itself.
//     Pays off on calm networks by reducing respawns later.
//
// No-op when no live session matches mediaID. Safe to call from any
// goroutine.
func (o *SegmentOrchestrator) NotifyClientHint(mediaItemID string, variantIdx, bufferLengthMs int, bufferStarved bool, segmentIdx int) {
	o.mu.Lock()
	matched := make([]*orchestratorSession, 0, 1)
	for _, list := range o.sessions {
		for _, s := range list {
			s.mu.Lock()
			mid := s.mediaID
			rung := s.variantIdx
			s.mu.Unlock()
			// Rung filter, only in SingleVariant mode: a multi-variant session
			// encodes every rung and leaves variantIdx at 0, where filtering would
			// silence the hint entirely. Applying a rung-0 hint to the abandoned
			// rung-2 session pushed its clientHighWater to the live playhead, so
			// evaluateThrottle's pause-when-ahead branch - the orchestrator's ONLY
			// brake - never fired again and the abandoned rung ran unthrottled.
			if mid != mediaItemID {
				continue
			}
			if o.opts.SingleVariant && rung != variantIdx {
				continue
			}
			matched = append(matched, s)
		}
	}
	o.mu.Unlock()
	if len(matched) == 0 {
		return
	}

	bufferAheadSegs := 0
	if bufferLengthMs > 0 && o.opts.SegmentSeconds > 0 {
		bufferAheadSegs = int(float64(bufferLengthMs) / 1000.0 / o.opts.SegmentSeconds)
	}
	// CMCD `bl` is client-reported and forgeable: a single hint with
	// bl=999999999 would otherwise stretch endSeg to encode the whole film below
	// (the MaxActiveSessions cap bounds the NUMBER of sessions, not their length).
	// Cap the prefetch reach at 2× the rafale lookahead.
	if maxAhead := 2 * o.opts.LookAheadSegments; bufferAheadSegs > maxAhead {
		bufferAheadSegs = maxAhead
	}
	requestedIdx := max(segmentIdx, 0)

	now := time.Now()
	for _, s := range matched {
		s.mu.Lock()
		// Clamp to just past this rafale's window: the handler's duration bound on
		// segmentIdx is conditional (init segments, copy rungs, unprobed items), so
		// a forged stream_0_999999.m4s reaches here, and clientHighWater only ever
		// moves forward - one such hint disabled the throttle for the whole session.
		consumableIdx := min(requestedIdx, s.endSeg+2*o.opts.LookAheadSegments)
		if consumableIdx > s.clientHighWater {
			s.clientHighWater = consumableIdx
		}
		// Resume throttle: at most one SIGCONT per second per session.
		// A client spamming `bs=1` (forged or genuinely terrible
		// network) can't burn through kernel syscalls under us.
		resumeAllowed := s.lastResumeAt.IsZero() || now.Sub(s.lastResumeAt) >= time.Second
		needsResume := bufferStarved && s.paused && resumeAllowed
		handle := s.handle

		target := consumableIdx + bufferAheadSegs + o.opts.LookAheadSegments
		if bufferAheadSegs >= o.opts.LookAheadSegments && target > s.endSeg {
			s.endSeg = target
		}
		s.mu.Unlock()

		if needsResume && handle != nil {
			if err := handle.Resume(); err != nil {
				o.logger.Warn("orchestrator: starvation resume failed",
					slog.String("media_id", mediaItemID), slog.Any("err", err))
				continue
			}
			s.mu.Lock()
			s.paused = false
			s.lastResumeAt = now
			s.mu.Unlock()
			o.logger.Info("orchestrator: throttle resumed by CMCD bs=1",
				slog.String("media_id", mediaItemID),
				slog.Int("segment_idx", segmentIdx))
			metrics.HLSVODThrottleResume.Inc()
			metrics.HLSVODStarvationResumes.Inc()
		}
	}
}
