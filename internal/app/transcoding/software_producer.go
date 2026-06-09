package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// SoftwareProducer is the software-transcode StreamProducer: the Jellyfin
// model. For a given DecisionHash it runs a small, bounded set of long-lived
// ffmpeg encoders, each input-paced with `-readrate` so it stays near
// wall-clock and never races the whole film ahead of the viewer. It serves a
// single rendition (the ladder is collapsed to one rung by Profiles), so there
// is nothing for the player to probe or switch across — one decode, one encode
// per live position, no ABR thrash.
//
// This is the antithesis of SegmentOrchestrator (the hardware path) on the axes
// that caused the weak-NAS famine: no burst-ahead across ABR rungs, no global
// capacity cap, no per-segment deadline kill. A weak NAS with hw_accel off
// serves a stable stream because the client WAITS for the paced encoder instead
// of abort-and-probing across rungs that each spawn their own starving session.
//
// It DOES, however, allow a SMALL bounded number of encoders for the SAME hash
// when (and only when) two viewers sit at genuinely different live positions of
// the same film — two browser tabs, or a PC + a phone, started at different
// offsets. Viewers at the same position share one encoder via the addressable
// cache; only distinct uncached positions get their own. This is the multi-tab
// coexistence the orchestrator already guarantees (commit 0ac96ebbb), ported to
// the software path: a second viewer NEVER kills the first's encoder.
//
// Lifecycle (invariants, per the decouple plan):
//  1. one paced decode-once ffmpeg per (Hash, live position); same-position
//     viewers share via the cache, distinct positions coexist up to
//     MaxSessionsPerHash (LRU-evicted past the cap)
//  2. ReadRate = 1.5 (input pacing)
//  3. never ErrCapacityExceeded, long WaitBudget (the client waits)
//  4. no NumCPU cap (only the per-hash MaxSessionsPerHash bound)
//  5. no per-segment deadline kill (idle GC only, 30 min)
//  6. Profiles() = single rung at the sustainable software ceiling
//  7. an uncovered position (e.g. a second viewer cold-starting below a deep
//     viewer) spawns a session ALONGSIDE — it never kills a sibling another
//     viewer is consuming; the LRU is evicted only when the cap is exceeded
//  8. the common VOD manifest is served alongside (the client reads segment N
//     when the paced process reaches it)
type SoftwareProducer struct {
	cache   *SegmentCache
	process transcoding.HLSProcess
	source  SourceResolver
	logger  *slog.Logger
	opts    SoftwareProducerOptions

	// copyPlanner gates COPY MODE (the Jellyfin-style paced video-copy rung).
	// nil = pure transcode, byte-identical to before — every copy-mode divergence
	// is behind a `copyPlanner != nil` check. When set (a dedicated copy instance
	// wired for the remux rung), startSession emits `-c:v copy` and maps a segment
	// index to its keyframe StartSec via the plan, and Manifest serves the
	// keyframe-aligned playlist. Set ONCE via SetCopyMode before first use.
	copyPlanner *CopyPlanner

	// shutdownCtx is the long-lived parent for every ffmpeg this producer
	// spawns — never the HTTP request context (that one dies when the segment
	// is served, which would tear down the long-lived encoder). Cancelled only
	// by Close.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	mu sync.Mutex
	// sessions maps a DecisionHash to its live software encoders. Usually one
	// (a single viewer, or several viewers sharing the cache at the same
	// position), but a small bounded slice when two viewers sit at genuinely
	// different live positions of the same film (two tabs / PC + phone started
	// at different offsets). A second viewer spawns a session ALONGSIDE rather
	// than killing the first's — the multi-tab coexistence guarantee. Bounded by
	// opts.MaxSessionsPerHash (LRU-evicted past the cap), mirroring the
	// orchestrator's []*orchestratorSession.
	sessions map[string][]*swSession
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

	// MaxSessionsPerHash bounds the number of concurrent ffmpeg encoders for a
	// SINGLE DecisionHash — i.e. how many genuinely-distinct live positions of
	// the same film coexist before the least-recently-used one is evicted.
	// Default 2: enough for the common two-tab / two-device case (the multi-tab
	// coexistence guarantee) while keeping concurrent software encodes on a weak
	// NAS bounded (each encode is full CPU; viewers at the same position share
	// one via the cache, so this only bites on distinct uncached positions). Set
	// to 1 to restore the legacy single-process-per-hash model (a second
	// distinct position then evicts the first — the pre-fix behaviour).
	MaxSessionsPerHash int

	// MaxConcurrentVariantsPerHash bounds how many DISTINCT rungs (variantIdx) may
	// have a live encoder for one film at once — the anti-thrash ceiling on a weak
	// NAS where each rung is its own full 4K decode (the software path never shares
	// a decode across rungs). Past the cap the least-recently-used rung's encoder is
	// evicted (its segments stay on disk for the LRU disk evictor). Default 2; set
	// to 1 on the weakest boxes so a level switch fully releases the old rung before
	// the new one runs. Distinct from MaxSessionsPerHash, which bounds TOTAL encoders
	// (including several positions of ONE rung); this bounds distinct rungs.
	MaxConcurrentVariantsPerHash int

	// SeekReachWindowSegs is how many segments AHEAD of a session's production
	// frontier a request may be and still reuse that (paced) encoder rather than
	// seek a fresh one. Within the window the player waits a beat for the
	// sequential encoder to arrive; beyond it (a real forward seek) waiting would
	// blow the wait budget — a 4K HDR encoder paced at 1.5× takes minutes to cross
	// hundreds of segments — so a fresh encoder seeks (`-ss`) straight to the
	// target. Default 16 (~48 s of 3 s segments): far above the 0-1 segment gap of
	// continuous playback (steady viewing never spuriously spawns a second encoder)
	// yet a small fraction of any deliberate scrub.
	SeekReachWindowSegs int
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
	if out.MaxSessionsPerHash <= 0 {
		out.MaxSessionsPerHash = 2
	}
	if out.MaxConcurrentVariantsPerHash <= 0 {
		out.MaxConcurrentVariantsPerHash = 2
	}
	if out.SeekReachWindowSegs <= 0 {
		out.SeekReachWindowSegs = 16
	}
	return out
}

// swSession is one long-lived ffmpeg encoder for a DecisionHash at a given
// start position. A hash may have several (one per distinct live position) so
// two viewers at different offsets coexist.
type swSession struct {
	mu      sync.Mutex
	hash    string
	mediaID string
	handle  transcoding.HLSProcessHandle
	// startSeg is the segment index the encoder was started at (`-ss`). With
	// `hls_list_size 0` ffmpeg produces forward from here indefinitely, so the
	// live window is [startSeg, +inf). A request below this session's startSeg
	// is not covered by it; the producer reuses a lower-start sibling if one
	// exists, else spawns a new session at the requested offset.
	startSeg int
	// variantIdx is which ABR rung (index into fp.Profiles) this encoder produces.
	// A session is only ever reused for a request of the SAME variantIdx — rungs
	// are isolated, because the cache Wait() polls per-variant disk files with no
	// per-variant pending channel, so handing a variant-0 encoder a variant-1
	// request would never write stream_1_* and rebuffer forever. Single-rendition
	// keeps this 0 and the whole producer byte-identical to before the ladder.
	variantIdx int
	// frontier is the high-water-mark segment index routed to this encoder (a
	// cache hit consumed from it, or an in-reach miss committed to it). It tracks
	// the playhead inside this session's forward run. A request more than
	// SeekReachWindowSegs ABOVE the frontier is a forward seek the paced sequential
	// encoder can't reach within the wait budget, so the producer seeks a FRESH
	// encoder there instead of making the player wait (or buffer forever). Guarded
	// by mu; initialised to startSeg.
	frontier   int
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
		sessions:       make(map[string][]*swSession),
		spawnedHashes:  make(map[string]struct{}),
		stopGC:         make(chan struct{}),
		gcDone:         make(chan struct{}),
	}
	go p.gcLoop()
	return p
}

// Profiles shapes the ABR ladder for the SOFTWARE path: the base ladder's rungs
// up to the tallest height the host sustains. Auto degrades DOWN this ladder
// under a stall and the viewer can lock a specific rung. It is safe on a weak box
// because the surrounding guards prevent the old multi-rung thrash: the producer
// caps how many distinct rungs decode at once (MaxConcurrentVariantsPerHash) and
// suspends an abandoned rung at once, while the client starts Auto at the top and
// only steps down (never probes up into a second concurrent decode). The top is
// the conservatively-calibrated sustainable ceiling, so it holds; the lowest rung
// (360p, present in every base ladder) is the guaranteed-playable floor.
//
//   - HDR (tonemap needed): capped at capacity.SoftwareHDRMaxHeight.
//   - SDR: capped at capacity.SoftwareMaxHeight.
//
// Pure in its inputs (base, src, decision, capacity) so the ladder is identical
// at manifest-time and segment-time — the cache-hash invariant.
// SetCopyMode enables the paced video-COPY rung on this producer instance (the
// Jellyfin-style remux: one paced -c:v copy session per live position, seek =
// restart). Call ONCE before first use — the wiring builds a DEDICATED copy
// instance for the remux path, distinct from the transcode instance the selector
// returns, so the two never share a manifest. nil planner = pure transcode.
func (p *SoftwareProducer) SetCopyMode(planner *CopyPlanner) {
	p.copyPlanner = planner
}

// Manifest serves the copy rendition's keyframe-aligned VOD media playlist
// (variable #EXTINF from the keyframe plan). Implements ManifestProvider.
//
// ⚠️ The type-assertion in serveVariant is STRUCTURAL — it matches *any*
// *SoftwareProducer, including the transcode instance (copyPlanner nil), NOT just
// a copy instance. So serveVariant MUST gate the ManifestProvider branch on
// isCopyRequest; without that gate the transcode path calls this and 500s (see
// the regression in commit history). This method returning an error when not in
// copy mode is only a backstop — the real guard is the handler's copy gate.
func (p *SoftwareProducer) Manifest(ctx context.Context, fp DecisionFingerprint) ([]byte, error) {
	if p.copyPlanner == nil {
		return nil, errors.New("software: Manifest unavailable (producer not in copy mode)")
	}
	segs, err := p.copyPlanner.Plan(ctx, fp.MediaID)
	if err != nil {
		return nil, fmt.Errorf("software: copy manifest for %s: %w", fp.MediaID, err)
	}
	return buildCopyManifest(segs), nil
}

func (p *SoftwareProducer) Profiles(base []transcoding.HLSProfile, src SourceDims, decision transcoding.PlaybackDecision, capacity transcoding.EncodeCapability) []transcoding.HLSProfile {
	// Copy mode = the Jellyfin remux: ONE rung at the source resolution with the
	// video copied (the handler advertises the source avc1.PPCCLL) and audio→AAC.
	// No transcode ladder — direct-play quality is the single rendition.
	if p.copyPlanner != nil {
		w, h := src.Width, src.Height
		if w <= 0 || h <= 0 {
			w, h = 1920, 1080 // safe default so the fingerprint stays deterministic
		}
		const copyDefaultBitrateKbps int64 = 5_000
		return []transcoding.HLSProfile{{
			Name:         fmt.Sprintf("%dp-copy", h),
			Width:        w,
			Height:       h,
			VideoBitrate: copyDefaultBitrateKbps,
			AudioBitrate: 192,
			Copy:         true, // folds into the cache fingerprint (copy vs transcode never alias)
		}}
	}

	sourceCapped := transcoding.CapLadderToSource(base, src.Width, src.Height)

	maxH := capacity.MaxHeightFor(false) // software ceiling (SoftwareMaxHeight or 720 floor)
	if needsHDRTonemap(decision) && capacity.SoftwareHDRMaxHeight > 0 && capacity.SoftwareHDRMaxHeight < maxH {
		maxH = capacity.SoftwareHDRMaxHeight
	}

	// The natural ladder capped to the sustainable ceiling — already ascending
	// with a 360p floor. No collapse: Auto needs the lower rungs to fall back to
	// and the viewer needs them to choose from.
	return transcoding.CapLadderToHeight(sourceCapped, maxH)
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
// Multi-position per hash (the multi-tab coexistence guarantee):
//   - cache hit: no ffmpeg, just bump the covering session's lastAccess
//   - a live session covers the request (its startSeg <= segmentIdx): reuse the
//     closest-behind one; its sequential encoder is already producing toward it
//   - no covering session (e.g. a second viewer cold-starting BELOW a deep
//     viewer's start): spawn a new encoder ALONGSIDE at segmentIdx — never kill
//     a sibling another viewer is consuming. Bounded by MaxSessionsPerHash; the
//     LRU is evicted only when that cap is exceeded.
func (p *SoftwareProducer) EnsureSegmentRange(ctx context.Context, fp DecisionFingerprint, variantIdx, segmentIdx int) error {
	if segmentIdx < 0 {
		return fmt.Errorf("software: EnsureSegmentRange: negative segmentIdx %d", segmentIdx)
	}
	hash := fp.Hash()

	// Fast path: already on disk.
	if p.cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: variantIdx, SegmentIdx: segmentIdx}, fp.UseFMP4) {
		p.touch(hash, variantIdx, segmentIdx)
		metrics.HLSVODCacheHits.Inc()
		return nil
	}
	metrics.HLSVODCacheMisses.Inc()

	p.mu.Lock()
	covering := p.coveringWithinReachLocked(hash, variantIdx, segmentIdx)
	p.mu.Unlock()

	if covering != nil {
		// A sibling encoder started at or below this position is already producing
		// forward toward segmentIdx AND is within reach of it; reuse it (resume if
		// the idle-pause loop suspended it — the client is pulling again). Commit
		// its frontier to the request so a run of just-in-time edge misses keeps
		// reusing this encoder instead of drifting past the reach window into a
		// spurious seek. No new process.
		covering.mu.Lock()
		covering.lastAccess = time.Now()
		if segmentIdx > covering.frontier {
			covering.frontier = segmentIdx
		}
		covering.mu.Unlock()
		p.resumeIfPaused(covering)
		return nil
	}

	// Nothing reaches this position in time: either no session covers it, or the
	// closest-behind one is a forward seek away (its frontier is too far below).
	// Start a fresh encoder seeking (`-ss`) to segmentIdx, ALONGSIDE any existing
	// sessions for the hash — we do NOT kill them. A second viewer cold-starting
	// below a deep viewer, or a viewer jumping forward past the live frontier, must
	// not yank a sibling another viewer consumes; they coexist, bounded by
	// MaxSessionsPerHash (LRU-evicted).
	return p.startSession(ctx, fp, variantIdx, segmentIdx)
}

// findCoveringSessionLocked returns the live session best placed to serve
// segmentIdx FOR THE GIVEN VARIANT: the same-variant session with the largest
// startSeg still <= segmentIdx (the closest-behind encoder, already producing
// forward toward it). Sessions producing a DIFFERENT rung are skipped — reusing
// one would never write this variant's segment file (the cache Wait polls
// per-variant), so the caller must spawn a fresh encoder for it. Returns nil when
// no same-variant session started at or below segmentIdx. Must hold p.mu.
func (p *SoftwareProducer) findCoveringSessionLocked(hash string, variantIdx, segmentIdx int) *swSession {
	var best *swSession
	bestStart := -1
	for _, s := range p.sessions[hash] {
		s.mu.Lock()
		sameVariant := s.variantIdx == variantIdx
		start := s.startSeg
		s.mu.Unlock()
		if sameVariant && start <= segmentIdx && start > bestStart {
			best = s
			bestStart = start
		}
	}
	return best
}

// startSession launches the one long-lived ffmpeg for hash at segmentIdx with
// input pacing. Single-rendition: encodes exactly fp.Profiles[variantIdx].
func (p *SoftwareProducer) startSession(ctx context.Context, fp DecisionFingerprint, variantIdx, startSeg int) error {
	hash := fp.Hash()

	if variantIdx < 0 || variantIdx >= len(fp.Profiles) {
		return fmt.Errorf("software: startSession: variantIdx %d out of range (profiles=%d)", variantIdx, len(fp.Profiles))
	}

	// Copy-mode divergences (gated). nil planner → transcode, byte-identical to
	// before. Resolved up front, before any resource is acquired, so a missing
	// plan returns cleanly. The frontier/seek/reach math downstream is in segment
	// COUNT, so only StartSec (the -ss offset) differs for variable-length copy
	// segments: segment N seeks to its keyframe start, not N×SegmentSeconds.
	videoDecision := "transcode"
	startSec := float64(startSeg) * p.opts.SegmentSeconds
	if p.copyPlanner != nil {
		segs, planErr := p.copyPlanner.Plan(ctx, fp.MediaID)
		if planErr != nil {
			return fmt.Errorf("software: startSession: copy plan for %s: %w", fp.MediaID, planErr)
		}
		if startSeg < 0 || startSeg >= len(segs) {
			return fmt.Errorf("software: startSession: copy startSeg %d out of range (%d segments)", startSeg, len(segs))
		}
		videoDecision = "copy"
		startSec = float64(segs[startSeg].StartMs) / 1000.0
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
		VideoDecision:      videoDecision,
		AudioDecision:      "transcode",
		AudioTrackIndex:    fp.AudioTrack,
		SubtitleTrackIndex: fp.SubtitleBurnInTrack,
		HDRSupported:       false, // tonemap is part of the decision; HDRSupported=false forces it
		ToneMap:            fp.HDRTonemap,
		AudioNormalize:     fp.AudioNormalize,
		StartSec:           startSec,
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
		variantIdx: variantIdx,
		startSeg:   startSeg,
		frontier:   startSeg,
		lastAccess: time.Now(),
		teardown:   teardown,
		cancelCtx:  cancel,
		done:       make(chan struct{}),
	}

	p.mu.Lock()
	// Race: another goroutine may have started a session reaching startSeg between
	// our coveringWithinReach decision and now. Re-check under the lock with the
	// SAME reach-aware test; if a sibling now reaches this exact position, discard
	// the loser (avoid a redundant duplicate encoder for the same offset). A
	// distant sibling that merely started BELOW startSeg does not count — that is
	// the forward-seek case we are spawning this fresh encoder for; distinct live
	// positions are the coexistence we want, not a race.
	if winner := p.coveringWithinReachLocked(hash, variantIdx, startSeg); winner != nil {
		p.mu.Unlock()
		cancel()
		if stopErr := handle.Stop(); stopErr != nil {
			p.logger.Debug("software: stop on start race lost (ignored)", slog.Any("err", stopErr))
		}
		teardown()
		return nil
	}
	p.sessions[hash] = append(p.sessions[hash], session)
	p.spawnedHashes[hash] = struct{}{}
	// Enforce the per-hash cap: if we just crossed MaxSessionsPerHash, evict the
	// least-recently-used sibling (never the one we just appended). Kill it
	// outside the lock — killSession reacquires p.mu.
	victim := p.evictLRUIfFullLocked(hash)
	variantVictims := p.evictExcessVariantsLocked(hash, variantIdx)
	p.mu.Unlock()

	if victim != nil {
		p.logger.Info("software: per-hash cap reached, evicting LRU encoder",
			slog.String("hash", hash), slog.Int("cap", p.opts.MaxSessionsPerHash))
		p.killSession(victim, "lru_cap")
	}
	for _, v := range variantVictims {
		p.logger.Info("software: distinct-variant cap reached, evicting LRU rung's encoder",
			slog.String("hash", hash), slog.Int("cap", p.opts.MaxConcurrentVariantsPerHash))
		p.killSession(v, "variant_cap")
	}

	// On a weak NAS only one heavy software transcode fits. This encoder just
	// started AHEAD of the viewer's previous position (a forward seek), so hand it
	// the CPU at once: SIGSTOP the hash's encoders strictly BEHIND it instead of
	// letting the abandoned one contend for up to PauseIdleThreshold seconds — a
	// deep forward seek otherwise ran the new encoder at ~0.2× realtime against the
	// old one, which is the "small buffering after seeking" symptom. Only behind
	// siblings are paused, so a deeper viewer / second tab AHEAD keeps running; a
	// paused sibling another viewer still consumes resumes on its next request.
	// Pause (not kill) preserves the multi-tab coexistence guarantee.
	p.pauseSiblingsBehind(hash, session)
	// And suspend any DIFFERENT-rung encoder for this film: a fresh rung means the
	// viewer switched levels, so the old rung's full 4K decode must not run
	// concurrently with the new rung on a weak box (the level-switch famine window).
	p.pauseOtherVariants(hash, session)

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
	for _, s := range matched {
		s.mu.Lock()
		s.lastAccess = time.Now()
		s.mu.Unlock()
		if starved {
			p.resumeIfPaused(s)
		}
	}
}

// touch bumps lastAccess on the session covering segmentIdx (cache-hit path) so
// the GC doesn't reap a session whose segments are still being consumed from
// disk, and resumes it if the idle-pause loop had suspended it. With several
// sessions per hash it bumps the closest-behind one (the encoder actually
// serving that region) rather than an unrelated sibling at a different offset.
func (p *SoftwareProducer) touch(hash string, variantIdx, segmentIdx int) {
	p.mu.Lock()
	s := p.findCoveringSessionLocked(hash, variantIdx, segmentIdx)
	p.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastAccess = time.Now()
	if segmentIdx > s.frontier {
		// A cache hit here proves the encoder has produced through segmentIdx, so
		// advance its frontier — keeps the seek-reach test (coveringWithinReach)
		// honest about how far this session has actually reached.
		s.frontier = segmentIdx
	}
	s.mu.Unlock()
	p.resumeIfPaused(s)
}

// coveringWithinReachLocked returns the live session that can serve segmentIdx
// within the wait budget: the closest-behind encoder (largest startSeg <=
// segmentIdx) whose production frontier is at most SeekReachWindowSegs behind the
// request. Returns nil when the best candidate is further behind — that's a
// forward seek, and reusing a sequential encoder that would take minutes to
// arrive is exactly what made deep seeks buffer forever; the caller instead
// seeks a FRESH encoder (`-ss`) to segmentIdx. Must be called with p.mu held.
func (p *SoftwareProducer) coveringWithinReachLocked(hash string, variantIdx, segmentIdx int) *swSession {
	s := p.findCoveringSessionLocked(hash, variantIdx, segmentIdx)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	point := max(s.startSeg, s.frontier)
	s.mu.Unlock()
	if segmentIdx-point > p.opts.SeekReachWindowSegs {
		return nil
	}
	return s
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
		s.paused = true
		h := s.handle
		hash := s.hash
		s.mu.Unlock()
		if err := h.Pause(); err != nil {
			s.mu.Lock()
			s.paused = false
			s.mu.Unlock()
			p.logger.Debug("software: pause failed", slog.String("hash", hash), slog.Any("err", err))
		}
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

// ActiveSessionCount returns how many software ffmpeg sessions are live across
// every hash (a hash may hold several when viewers sit at different positions).
func (p *SoftwareProducer) ActiveSessionCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, list := range p.sessions {
		n += len(list)
	}
	return n
}

// RemoveSession kills EVERY session producing for the given media ID — across
// all hashes (different audio/sub/codec decisions) and all positions. This is
// the FORCEFUL path (admin kill-session): it tears sessions down regardless of
// whether another viewer is consuming them. The gentle playback-stop path must
// NOT call this unconditionally — the StopStream handler gates it on "no other
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
	p.sessions[hash] = append(list[:victimIdx], list[victimIdx+1:]...)
	return victim
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
	for _, s := range list {
		s.mu.Lock()
		la, v := s.lastAccess, s.variantIdx
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
			if v == keepVariant {
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

// Compile-time assertion: SoftwareProducer satisfies the StreamProducer seam.
var _ StreamProducer = (*SoftwareProducer)(nil)
