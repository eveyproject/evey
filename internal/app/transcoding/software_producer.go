package transcoding

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// SoftwareProducer is the software-transcode StreamProducer: the Jellyfin
// model. For a given DecisionHash it runs ONE long-lived ffmpeg, input-paced
// with `-readrate` so it stays near wall-clock and never races the whole film
// ahead of the viewer. It serves a single rendition (the ladder is collapsed
// to one rung by Profiles), so there is nothing for the player to probe or
// switch across — one decode, one encode, no thrash.
//
// This is the antithesis of SegmentOrchestrator (the hardware path): no
// multi-session burst-ahead, no global capacity cap, no per-segment deadline
// kill. A weak NAS with hw_accel off serves a stable stream because the client
// WAITS for the paced encoder instead of abort-and-probing across rungs that
// each spawn their own starving session.
//
// Lifecycle (8 invariants, per the decouple plan):
//  1. one sequential decode-once ffmpeg per Hash (no spawnSession / slot acquire)
//  2. ReadRate = 1.5 (input pacing)
//  3. never ErrCapacityExceeded, long WaitBudget (the client waits)
//  4. no NumCPU cap
//  5. no per-segment deadline kill (idle GC only, 30 min)
//  6. Profiles() = single rung at the sustainable software ceiling
//  7. a backward seek (request before the live window) kills + restarts the one process
//  8. the common VOD manifest is served alongside (the client reads segment N
//     when the paced process reaches it)
type SoftwareProducer struct {
	cache   *SegmentCache
	process transcoding.HLSProcess
	source  SourceResolver
	logger  *slog.Logger
	opts    SoftwareProducerOptions

	// shutdownCtx is the long-lived parent for every ffmpeg this producer
	// spawns — never the HTTP request context (that one dies when the segment
	// is served, which would tear down the long-lived encoder). Cancelled only
	// by Close.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	mu sync.Mutex
	// sessions maps a DecisionHash to its single live software session. Unlike
	// the orchestrator's []*orchestratorSession (multi-rafale), the software
	// model is strictly one-process-per-hash — a backward seek replaces it.
	sessions map[string]*swSession
	// spawnedHashes tracks every hash a session was ever spawned for so Close
	// can purge the disk cache even after the GC swept the live entry.
	spawnedHashes map[string]struct{}

	prewarmWG sync.WaitGroup

	stopGC chan struct{}
	gcDone chan struct{}
}

// SoftwareProducerOptions configures the software producer's pacing and
// lifecycle. Zero values get sane defaults via withDefaults.
type SoftwareProducerOptions struct {
	// ReadRate paces ffmpeg's input to N× real-time (`-readrate N`). Default
	// 1.5: one sequential encoder kept near wall-clock so it doesn't burn CPU
	// racing ahead of a viewer who is, by construction, watching in real time.
	ReadRate float64

	// SegmentSeconds — keep in sync with ManifestSegmentSeconds and the ffmpeg
	// keyframe cadence. Maps a requested segment index to a `-ss` seek offset.
	SegmentSeconds float64

	// InactivityTimeout is how long a session lingers with no segment request
	// before the GC kills it AND frees its disk segments. Default 30 min — long,
	// matching the legacy HLSManager (a Chromecast can sit on a deep buffer for
	// minutes), because the LRU disk evictor bounds the disk cost independently.
	InactivityTimeout time.Duration

	// GCInterval is the period of the idle sweeper. Default 1 min.
	GCInterval time.Duration

	// PauseIdleThreshold is how long with no segment fetch before ffmpeg is
	// SIGSTOPped (it keeps the paced encoder from running far past a paused
	// player). A segment request resumes it. Default 20 s.
	PauseIdleThreshold time.Duration

	// WaitBudgetDur is what WaitBudget() reports — how long the handler blocks
	// on cache.Wait for a software segment. Default 60 s: a single paced encoder
	// is slower to land segment 0 on a 4K HDR software decode, and the client is
	// expected to wait (no abort-and-probe).
	WaitBudgetDur time.Duration
}

func (o SoftwareProducerOptions) withDefaults() SoftwareProducerOptions {
	out := o
	if out.ReadRate <= 0 {
		out.ReadRate = 1.5
	}
	if out.SegmentSeconds <= 0 {
		out.SegmentSeconds = ManifestSegmentSeconds
	}
	if out.InactivityTimeout <= 0 {
		out.InactivityTimeout = 30 * time.Minute
	}
	if out.GCInterval <= 0 {
		out.GCInterval = time.Minute
	}
	if out.PauseIdleThreshold <= 0 {
		out.PauseIdleThreshold = 20 * time.Second
	}
	if out.WaitBudgetDur <= 0 {
		out.WaitBudgetDur = 60 * time.Second
	}
	return out
}

// swSession is the single long-lived ffmpeg for one DecisionHash.
type swSession struct {
	mu      sync.Mutex
	hash    string
	mediaID string
	handle  transcoding.HLSProcessHandle
	// startSeg is the segment index the encoder was started at (`-ss`). With
	// `hls_list_size 0` ffmpeg produces forward from here indefinitely, so the
	// live window is [startSeg, +inf). A request below startSeg is a backward
	// seek that restarts the process at the new offset.
	startSeg   int
	lastAccess time.Time
	paused     bool
	teardown   func()
	cancelCtx  context.CancelFunc
	done       chan struct{}
}

// NewSoftwareProducer wires the cache + ffmpeg adapter + source resolver into a
// software producer. Owns a goroutine sweeping idle sessions; call Close on
// shutdown to stop it cleanly.
func NewSoftwareProducer(
	cache *SegmentCache,
	process transcoding.HLSProcess,
	source SourceResolver,
	opts SoftwareProducerOptions,
	logger *slog.Logger,
) *SoftwareProducer {
	if logger == nil {
		logger = slog.Default()
	}
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	p := &SoftwareProducer{
		cache:          cache,
		process:        process,
		source:         source,
		logger:         logger.With("component", "software_producer"),
		opts:           opts.withDefaults(),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		sessions:       make(map[string]*swSession),
		spawnedHashes:  make(map[string]struct{}),
		stopGC:         make(chan struct{}),
		gcDone:         make(chan struct{}),
	}
	go p.gcLoop()
	return p
}

// Profiles shapes the ABR ladder for the SOFTWARE path: ONE rung at the
// tallest height the host sustains for a single on-demand encode. A weak box
// cannot run the parallel ladder the player would probe/switch across — that
// thrashes — so we collapse to one rung (the Jellyfin single-rendition model).
//
// The rung height is the sustainable software ceiling:
//   - HDR (tonemap needed): capacity.SoftwareHDRMaxHeight (the honest 10-bit
//     decode + zscale tonemap ceiling on a weak NAS). It is PROVEN the old iGPU
//     cannot hold 1080p HDR software — that timed out with no fallback — so we
//     do NOT inflate it here; 1080p HDR comes later via the tonemap optimisation.
//   - SDR: capacity.SoftwareMaxHeight.
//
// Pure in its inputs (base, src, decision, capacity) so the ladder is identical
// at manifest-time and segment-time — the cache-hash invariant.
func (p *SoftwareProducer) Profiles(base []transcoding.HLSProfile, src SourceDims, decision transcoding.PlaybackDecision, capacity transcoding.EncodeCapability) []transcoding.HLSProfile {
	sourceCapped := transcoding.CapLadderToSource(base, src.Width, src.Height)

	maxH := capacity.MaxHeightFor(false) // software ceiling (SoftwareMaxHeight or 720 floor)
	if needsHDRTonemap(decision) && capacity.SoftwareHDRMaxHeight > 0 && capacity.SoftwareHDRMaxHeight < maxH {
		maxH = capacity.SoftwareHDRMaxHeight
	}

	laddered := transcoding.CapLadderToHeight(sourceCapped, maxH)
	if len(laddered) == 0 {
		return laddered
	}
	// Collapse to the single tallest sustainable rung.
	top := laddered[0]
	for _, prof := range laddered[1:] {
		if prof.Height > top.Height {
			top = prof
		}
	}
	return []transcoding.HLSProfile{top}
}

// needsHDRTonemap reports whether the decision flagged HDR->SDR tone-mapping.
// Mirrors the handler's hdr_tonemapping_needed reason check.
func needsHDRTonemap(decision transcoding.PlaybackDecision) bool {
	return slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
}

// WaitBudget reports how long the handler should block on cache.Wait for a
// software segment. Long by design: the single paced encoder is slower to land
// segment 0 and the client waits rather than abort-and-probe.
func (p *SoftwareProducer) WaitBudget() time.Duration { return p.opts.WaitBudgetDur }

// EnsureSegmentRange guarantees segment `segmentIdx` will be produced (or is
// already on disk). Returns immediately; the caller blocks on cache.Wait.
//
// NEVER returns ErrCapacityExceeded — the software model has no global session
// cap. A second media never sheds the first: both get their own long-lived
// encoder, paced so the host degrades gracefully under the input-rate cap
// rather than 503-looping.
//
// Session matching is strictly one-per-hash:
//   - cache hit: no ffmpeg, just bump lastAccess
//   - live session covers the request (segmentIdx >= startSeg): reuse, the
//     sequential encoder is already producing forward toward it
//   - backward seek (segmentIdx < startSeg) or no session: kill any existing
//     session and (re)start one ffmpeg at segmentIdx
func (p *SoftwareProducer) EnsureSegmentRange(ctx context.Context, fp DecisionFingerprint, variantIdx, segmentIdx int) error {
	if segmentIdx < 0 {
		return fmt.Errorf("software: EnsureSegmentRange: negative segmentIdx %d", segmentIdx)
	}
	hash := fp.Hash()

	// Fast path: already on disk.
	if p.cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: variantIdx, SegmentIdx: segmentIdx}, fp.UseFMP4) {
		p.touch(hash)
		metrics.HLSVODCacheHits.Inc()
		return nil
	}
	metrics.HLSVODCacheMisses.Inc()

	p.mu.Lock()
	existing := p.sessions[hash]
	p.mu.Unlock()

	if existing != nil {
		existing.mu.Lock()
		covered := segmentIdx >= existing.startSeg
		if covered {
			existing.lastAccess = time.Now()
		}
		existing.mu.Unlock()
		if covered {
			// The single sequential encoder is producing forward from startSeg;
			// the requested segment is in its window (or imminently will be).
			// Resume it if the idle-pause loop had suspended it — the client is
			// pulling again.
			p.resumeIfPaused(existing)
			return nil
		}
		// Backward seek: the requested segment is before the live window. Kill
		// the one process and restart it at the new offset (the legacy
		// RestartFromPosition model — software can't cheaply random-access).
		p.logger.Info("software: backward seek, restarting single process",
			slog.String("hash", hash),
			slog.Int("requested", segmentIdx),
			slog.Int("live_start", existing.startSeg))
		p.killSession(existing, "seek")
	}

	return p.startSession(ctx, fp, variantIdx, segmentIdx)
}

// startSession launches the one long-lived ffmpeg for hash at segmentIdx with
// input pacing. Single-rendition: encodes exactly fp.Profiles[variantIdx].
func (p *SoftwareProducer) startSession(ctx context.Context, fp DecisionFingerprint, variantIdx, startSeg int) error {
	hash := fp.Hash()

	if variantIdx < 0 || variantIdx >= len(fp.Profiles) {
		return fmt.Errorf("software: startSession: variantIdx %d out of range (profiles=%d)", variantIdx, len(fp.Profiles))
	}

	srcPath, _, err := p.source.ResolveSource(ctx, fp)
	if err != nil {
		return fmt.Errorf("software: startSession: resolve source: %w", err)
	}

	outDir, err := p.cache.DirFor(hash)
	if err != nil {
		return fmt.Errorf("software: startSession: cache dir: %w", err)
	}

	teardown := p.cache.RegisterSession(hash)

	procOpts := transcoding.HLSProcessOpts{
		OutputDir:          outDir,
		FilePath:           srcPath,
		Profiles:           profilesToDomain(fp.Profiles[variantIdx : variantIdx+1]),
		VariantIndex:       variantIdx,
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		AudioTrackIndex:    fp.AudioTrack,
		SubtitleTrackIndex: fp.SubtitleBurnInTrack,
		HDRSupported:       false, // tonemap is part of the decision; HDRSupported=false forces it
		ToneMap:            fp.HDRTonemap,
		AudioNormalize:     fp.AudioNormalize,
		StartSec:           float64(startSeg) * p.opts.SegmentSeconds,
		// Input pacing: the defining software-flow knob. Keeps the single
		// sequential encoder near wall-clock so it never races ahead of the
		// viewer (no abort-and-probe client → no need to burst).
		ReadRate:         p.opts.ReadRate,
		VideoCodec:       fp.VideoCodec,
		AudioCodec:       fp.AudioCodec,
		MaxAudioChannels: fp.MaxAudioChannels,
		ForceSidecarSubs: fp.ForceSidecarSubs,
		UseFMP4:          fp.UseFMP4,
		// HWAccel deliberately left empty — this is the SOFTWARE producer. Even
		// if the fingerprint carried a HW backend (it won't, the selector only
		// routes here when HW is unusable), we never run the GPU pipeline here.
	}

	sessionCtx, cancel := context.WithCancel(p.shutdownCtx)
	handle, err := p.process.Start(sessionCtx, procOpts)
	if err != nil {
		cancel()
		teardown()
		return fmt.Errorf("software: startSession: ffmpeg start: %w", err)
	}

	session := &swSession{
		hash:       hash,
		mediaID:    fp.MediaID,
		handle:     handle,
		startSeg:   startSeg,
		lastAccess: time.Now(),
		teardown:   teardown,
		cancelCtx:  cancel,
		done:       make(chan struct{}),
	}

	p.mu.Lock()
	// Race: another goroutine may have started a session for this hash between
	// our unlock and now. One-process-per-hash — discard the loser.
	if winner := p.sessions[hash]; winner != nil {
		p.mu.Unlock()
		cancel()
		if stopErr := handle.Stop(); stopErr != nil {
			p.logger.Debug("software: stop on start race lost (ignored)", slog.Any("err", stopErr))
		}
		teardown()
		return nil
	}
	p.sessions[hash] = session
	p.spawnedHashes[hash] = struct{}{}
	p.mu.Unlock()

	// Reap when ffmpeg exits so the live entry doesn't linger and the cache
	// pending refcount is released (a natural EOF skips killSession's teardown).
	go func() {
		select {
		case <-handle.Done():
		case <-sessionCtx.Done():
		}
		close(session.done)
		session.mu.Lock()
		td := session.teardown
		session.teardown = nil
		session.mu.Unlock()
		if td != nil {
			td()
		}
		p.removeFromMap(hash, session)
	}()

	p.logger.Info("software: session started",
		slog.String("hash", hash),
		slog.Int("start_seg", startSeg),
		slog.Float64("read_rate", p.opts.ReadRate),
		slog.String("source", srcPath))
	metrics.HLSVODRafaleSpawned.WithLabelValues("software").Inc()
	metrics.HLSVODSessions.Inc()
	return nil
}

// PreWarm starts the one software ffmpeg for fp in the background (cold-start
// optimisation: kick the slow software decode off before the player asks for
// segment 0). Bounded by shutdownCtx so Close cancels in-flight pre-warms.
func (p *SoftwareProducer) PreWarm(fp DecisionFingerprint, startSegIdx int) {
	if p.shutdownCtx.Err() != nil {
		return
	}
	p.prewarmWG.Go(func() {
		ctx, cancel := context.WithTimeout(p.shutdownCtx, 30*time.Second)
		defer cancel()
		if err := p.EnsureSegmentRange(ctx, fp, 0, startSegIdx); err != nil {
			p.logger.Debug("software: prewarm failed (non-fatal)",
				slog.String("media_id", fp.MediaID), slog.Any("err", err))
		}
	})
}

// NotifyClientHint resumes a paused encoder when the client reports starvation,
// and bumps lastAccess so the GC keeps the session warm. It NEVER changes which
// variant is served (there is only one rung anyway). The single paced encoder
// has no burst-ahead window to stretch, so a fat-buffer hint is a no-op beyond
// keeping the session alive.
func (p *SoftwareProducer) NotifyClientHint(mediaItemID string, bufferLengthMs int, starved bool, segmentIdx int) {
	_ = bufferLengthMs
	_ = segmentIdx
	p.mu.Lock()
	var matched []*swSession
	for _, s := range p.sessions {
		s.mu.Lock()
		mid := s.mediaID
		s.mu.Unlock()
		if mid == mediaItemID {
			matched = append(matched, s)
		}
	}
	p.mu.Unlock()
	for _, s := range matched {
		s.mu.Lock()
		s.lastAccess = time.Now()
		s.mu.Unlock()
		if starved {
			p.resumeIfPaused(s)
		}
	}
}

// touch bumps lastAccess on the session for hash (cache-hit path) so the GC
// doesn't reap a session whose segments are still being consumed from disk, and
// resumes it if the idle-pause loop had suspended it.
func (p *SoftwareProducer) touch(hash string) {
	p.mu.Lock()
	s := p.sessions[hash]
	p.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastAccess = time.Now()
	s.mu.Unlock()
	p.resumeIfPaused(s)
}

// resumeIfPaused SIGCONTs the encoder if the idle-pause loop had suspended it.
// Idempotent on a running process; only fires on the paused→active edge.
func (p *SoftwareProducer) resumeIfPaused(s *swSession) {
	s.mu.Lock()
	if !s.paused || s.handle == nil {
		s.mu.Unlock()
		return
	}
	s.paused = false
	h := s.handle
	s.mu.Unlock()
	if err := h.Resume(); err != nil {
		p.logger.Warn("software: resume ffmpeg failed", slog.String("hash", s.hash), slog.Any("err", err))
	}
}

// ActiveSessionCount returns how many software ffmpeg sessions are live.
func (p *SoftwareProducer) ActiveSessionCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sessions)
}

// RemoveSession kills the session producing for the given media ID (a single
// media can have several hashes — different audio/sub/codec decisions — so we
// fan out). No-op when none match.
func (p *SoftwareProducer) RemoveSession(mediaItemID string) {
	p.mu.Lock()
	var toKill []*swSession
	for _, s := range p.sessions {
		s.mu.Lock()
		match := s.mediaID == mediaItemID
		s.mu.Unlock()
		if match {
			toKill = append(toKill, s)
		}
	}
	p.mu.Unlock()
	for _, s := range toKill {
		p.killSession(s, "remove_session")
	}
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
	metrics.HLSVODSessions.Dec()
}

// removeFromMap drops the session from the live map iff it is still the current
// one for hash (a restart may have replaced it).
func (p *SoftwareProducer) removeFromMap(hash string, target *swSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions[hash] == target {
		delete(p.sessions, hash)
	}
}

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
	for _, s := range p.sessions {
		s.mu.Lock()
		idle := s.lastAccess.Before(cutoff)
		s.mu.Unlock()
		if idle {
			toKill = append(toKill, s)
		}
	}
	p.mu.Unlock()
	for _, s := range toKill {
		hash := s.hash
		p.killSession(s, "inactivity")
		if err := p.cache.CleanupHash(hash); err != nil {
			p.logger.Warn("software: idle cleanup failed", slog.String("hash", hash), slog.Any("err", err))
		}
	}
}

// sweepPause SIGSTOPs the encoder for any session whose client has gone quiet
// past PauseIdleThreshold. A segment fetch (touch / EnsureSegmentRange) or a
// CMCD starvation hint resumes it. Saves CPU on a paused player without
// touching the (no-eviction) playlist.
func (p *SoftwareProducer) sweepPause() {
	cutoff := time.Now().Add(-p.opts.PauseIdleThreshold)
	p.mu.Lock()
	snapshot := make([]*swSession, 0, len(p.sessions))
	for _, s := range p.sessions {
		snapshot = append(snapshot, s)
	}
	p.mu.Unlock()
	for _, s := range snapshot {
		s.mu.Lock()
		if s.paused || s.handle == nil || !s.lastAccess.Before(cutoff) {
			s.mu.Unlock()
			continue
		}
		s.paused = true
		h := s.handle
		hash := s.hash
		s.mu.Unlock()
		if err := h.Pause(); err != nil {
			s.mu.Lock()
			s.paused = false
			s.mu.Unlock()
			p.logger.Debug("software: pause ffmpeg failed", slog.String("hash", hash), slog.Any("err", err))
			continue
		}
		p.logger.Debug("software: ffmpeg suspended — client idle", slog.String("hash", hash))
	}
}

// Close stops the GC, kills every live session, and purges every spawned hash's
// disk segments. Safe to call once.
func (p *SoftwareProducer) Close() {
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
	p.sessions = make(map[string]*swSession)
	p.spawnedHashes = make(map[string]struct{})
	p.mu.Unlock()

	for _, hash := range hashes {
		if s := live[hash]; s != nil {
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

// Compile-time assertion: SoftwareProducer satisfies the StreamProducer seam.
var _ StreamProducer = (*SoftwareProducer)(nil)
