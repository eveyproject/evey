package transcoding

// software_producer_window.go bounds what a COPY session leaves on disk. The
// remux runs at 8x real time and used to keep every segment from its start to
// the end of the film, so one viewer of a 70 GB MKV meant 70 GB of cache
// within a quarter of an hour, on the system disk of a small box. A copy
// session now keeps a window around the viewer: it is SIGSTOPped once it runs
// CopyLeadSegs ahead of the last segment served, resumed when the viewer has
// eaten half of that lead, and the segments more than CopyKeepBehindSegs
// behind the viewer are deleted. A seek outside the window is a cache miss the
// producer already knows how to serve: a fresh `-ss` remux at the keyframe,
// which lands in about a second. Transcode sessions are untouched: they are
// paced near real time and reclaimed minutes after the viewer leaves.

import (
	"log/slog"
	"os"
	"time"
)

// sweepCopyWindow runs on the pause ticker for the copy instance only.
func (p *SoftwareProducer) sweepCopyWindow() {
	if p.copyPlanner == nil {
		return
	}
	snapshot := func() map[string][]*swSession {
		p.mu.Lock()
		defer p.mu.Unlock()
		out := make(map[string][]*swSession, len(p.sessions))
		for hash, list := range p.sessions {
			out[hash] = append([]*swSession(nil), list...)
		}
		return out
	}()
	for hash, list := range snapshot {
		p.trimBehindViewers(hash, list)
		for _, s := range list {
			p.capLead(s)
		}
	}
}

// capLead pauses a session that produced further than CopyLeadSegs beyond its
// viewer's position, and resumes a lead-paused one once the viewer is within
// half the lead. The pause is its own flag, distinct from the idle pause: a
// segment request must not resume it (the viewer pulling is exactly the case
// where the lead is still full), only this sweep or a starvation hint does.
func (p *SoftwareProducer) capLead(s *swSession) {
	s.mu.Lock()
	hash, variant, frontier, useFMP4 := s.hash, s.variantIdx, s.frontier, s.useFMP4
	dead := s.cancelCtx == nil || s.handle == nil
	s.mu.Unlock()
	if dead {
		return
	}
	lead := p.opts.CopyLeadSegs
	produced := p.cache.HighestReadyFrom(hash, variant, useFMP4, frontier+1, frontier+2*lead)
	ahead := produced - frontier
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !s.paused && ahead > lead:
		if err := s.handle.Pause(); err != nil {
			p.logger.Debug("software: copy lead pause failed", slog.String("hash", hash), slog.Any("err", err))
			return
		}
		s.paused, s.leadPaused, s.pausedSince = true, true, time.Now()
		p.logger.Debug("software: copy session suspended - far enough ahead of the viewer",
			slog.String("hash", hash), slog.Int("frontier", frontier), slog.Int("produced", produced))
	case s.leadPaused && ahead <= lead/2:
		if err := s.handle.Resume(); err != nil {
			p.logger.Warn("software: copy lead resume failed", slog.String("hash", hash), slog.Any("err", err))
			return
		}
		s.paused, s.leadPaused, s.pausedSince = false, false, time.Time{}
	}
}

// trimBehindViewers deletes, per rung, the segments more than CopyKeepBehindSegs
// behind the lowest viewer position among the hash's live sessions, and raises
// each session's floor so a request below it is a miss (a fresh session) rather
// than a wait on a file nobody will write again.
func (p *SoftwareProducer) trimBehindViewers(hash string, list []*swSession) {
	type rungView struct {
		lowest  int
		useFMP4 bool
	}
	lowest := map[int]rungView{}
	for _, s := range list {
		s.mu.Lock()
		v, ok := lowest[s.variantIdx]
		if !ok || s.frontier < v.lowest {
			lowest[s.variantIdx] = rungView{lowest: s.frontier, useFMP4: s.useFMP4}
		}
		s.mu.Unlock()
	}
	for variant, v := range lowest {
		cut := v.lowest - p.opts.CopyKeepBehindSegs
		if cut <= 0 {
			continue
		}
		ready, err := p.cache.ListReady(hash, variant, v.useFMP4)
		if err != nil {
			continue
		}
		removed := 0
		for _, idx := range ready {
			if idx >= cut {
				break // sorted ascending
			}
			path := p.cache.Path(SegmentKey{DecisionHash: hash, VariantIdx: variant, SegmentIdx: idx}, v.useFMP4)
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
		for _, s := range list {
			s.mu.Lock()
			if s.variantIdx == variant && cut > s.floorSeg {
				s.floorSeg = cut
			}
			s.mu.Unlock()
		}
		if removed > 0 {
			p.logger.Debug("software: copy segments trimmed behind the viewer",
				slog.String("hash", hash), slog.Int("variant", variant), slog.Int("below", cut), slog.Int("removed", removed))
		}
	}
}
