package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// SoftwareProducer is the software-transcode StreamProducer: the classic
// model. For a given DecisionHash it runs a small, bounded set of long-lived
// ffmpeg encoders, each input-paced with `-readrate` so it stays near
// wall-clock and never races the whole film ahead of the viewer.
//
// Profiles serves a ladder CAPPED at the sustainable software ceiling (a single
// rung in copy mode). The transcode path keeps the lower rungs so Auto can fall
// back to a smaller rendition when the top is slow - but the famine the decouple
// fixed (N unpaced parallel encoders) is held off a different way than forced
// single-rung: at most MaxConcurrentVariantsPerHash encoders run per hash at
// once, and requesting a variant SIGSTOPs the siblings (pauseOtherVariants), so
// only the rung the client is actually pulling burns CPU. The client never gets
// ErrCapacityExceeded - it WAITS for the paced encoder rather than abort-probing
// across rungs that each spawn their own starving session.
//
// This is the antithesis of SegmentOrchestrator (the hardware path) on the axes
// that caused the weak-NAS famine: no burst-ahead across ABR rungs, no global
// capacity cap, no per-segment deadline kill. A weak NAS with hw_accel off
// serves a stable stream because the client WAITS for the paced encoder instead
// of abort-and-probing across rungs that each spawn their own starving session.
//
// It DOES, however, allow a SMALL bounded number of encoders for the SAME hash
// when (and only when) two viewers sit at genuinely different live positions of
// the same film - two browser tabs, or a PC + a phone, started at different
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
//  6. Profiles() = ladder capped at the sustainable software ceiling (single
//     rung in copy mode); concurrency held by MaxConcurrentVariantsPerHash +
//     SIGSTOP of the non-requested variants, not by collapsing to one rung
//  7. an uncovered position (e.g. a second viewer cold-starting below a deep
//     viewer) spawns a session ALONGSIDE - it never kills a sibling another
//     viewer is consuming; the LRU is evicted only when the cap is exceeded
//  8. the common VOD manifest is served alongside (the client reads segment N
//     when the paced process reaches it)
type SoftwareProducer struct {
	cache   *SegmentCache
	process transcoding.HLSProcess
	source  SourceResolver
	logger  *slog.Logger
	opts    SoftwareProducerOptions

	// copyPlanner gates COPY MODE (the paced video-copy remux rung).
	// nil = pure transcode, byte-identical to before - every copy-mode divergence
	// is behind a `copyPlanner != nil` check. When set (a dedicated copy instance
	// wired for the remux rung), startSession emits `-c:v copy` and maps a segment
	// index to its keyframe StartSec via the plan, and Manifest serves the
	// keyframe-aligned playlist. Set ONCE via SetCopyMode before first use.
	copyPlanner *CopyPlanner

	// seekProber corrects a mid-stream copy session's start for the container's
	// REAL seek landing (sparse cue index → -ss lands on the preceding cue
	// point, possibly GOPs early). nil = no correction. Set via SetCopyMode.
	seekProber SeekLandingProber

	// videoLimiter caps the number of concurrently-ENCODING video sessions this
	// producer instance may run, shared with the hardware SegmentOrchestrator via
	// the same *VideoSessionLimiter. nil = unwired = unlimited (byte-identical to
	// the pre-feature behaviour). Only consulted when copyPlanner == nil - a copy
	// (remux) session never encodes video and must never be gated here. Set via
	// WithVideoSessionLimiter before first use.
	videoLimiter *VideoSessionLimiter

	// shutdownCtx is the long-lived parent for every ffmpeg this producer
	// spawns - never the HTTP request context (that one dies when the segment
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
	// than killing the first's - the multi-tab coexistence guarantee. Bounded by
	// opts.MaxSessionsPerHash (LRU-evicted past the cap), mirroring the
	// orchestrator's []*orchestratorSession.
	sessions map[string][]*swSession
	// spawnedHashes tracks every hash a session was ever spawned for so Close
	// can purge the disk cache even after the GC swept the live entry.
	spawnedHashes map[string]struct{}

	prewarmWG sync.WaitGroup

	stopGC chan struct{}
	gcDone chan struct{}

	// closeOnce guards Close so a double shutdown doesn't panic on a
	// re-close of stopGC.
	closeOnce sync.Once
}

// SoftwareProducerOptions configures the software producer's pacing and
// lifecycle. Zero values get sane defaults via withDefaults.
type SoftwareProducerOptions struct {
	// ReadRate paces ffmpeg's input to N× real-time (`-readrate N`). Default
	// 1.5: one sequential encoder kept near wall-clock so it doesn't burn CPU
	// racing ahead of a viewer who is, by construction, watching in real time.
	ReadRate float64

	// SegmentSeconds - keep in sync with ManifestSegmentSeconds and the ffmpeg
	// keyframe cadence. Maps a requested segment index to a `-ss` seek offset.
	SegmentSeconds float64

	// InactivityTimeout is how long a session lingers with no segment request
	// before the GC kills it AND frees its disk segments. Default 30 min - long,
	// matching the legacy HLSManager (a Chromecast can sit on a deep buffer for
	// minutes), because the LRU disk evictor bounds the disk cost independently.
	InactivityTimeout time.Duration

	// GCInterval is the period of the idle sweeper. Default 1 min.
	GCInterval time.Duration

	// PauseIdleThreshold is how long with no segment fetch before ffmpeg is
	// SIGSTOPped (it keeps the paced encoder from running far past a paused
	// player). A segment request resumes it. Default 20 s.
	PauseIdleThreshold time.Duration

	// WaitBudgetDur is what WaitBudget() reports - how long the handler blocks
	// on cache.Wait for a software segment. Default 60 s: a single paced encoder
	// is slower to land segment 0 on a 4K HDR software decode, and the client is
	// expected to wait (no abort-and-probe).
	WaitBudgetDur time.Duration

	// MaxSessionsPerHash bounds the number of concurrent ffmpeg encoders for a
	// SINGLE DecisionHash - i.e. how many genuinely-distinct live positions of
	// the same film coexist before the least-recently-used one is evicted.
	// Default 2: enough for the common two-tab / two-device case (the multi-tab
	// coexistence guarantee) while keeping concurrent software encodes on a weak
	// NAS bounded (each encode is full CPU; viewers at the same position share
	// one via the cache, so this only bites on distinct uncached positions). Set
	// to 1 to restore the legacy single-process-per-hash model (a second
	// distinct position then evicts the first - the pre-fix behaviour).
	MaxSessionsPerHash int

	// NewbornEvictionShield is how long a freshly-spawned encoder that has NOT
	// yet delivered its first segment is protected from cap eviction
	// (MaxSessionsPerHash / MaxConcurrentVariantsPerHash).
	//
	// A newborn is exactly what an active SegmentCache.Wait depends on: killing
	// it turns overload into LIVELOCK. On 2026-07-15 a player blocked on one
	// rung's segment 0 watched its encoder die to a sibling's spawn, retried,
	// respawned it - which evicted the sibling, whose waiter retried in turn.
	// Every encoder was killed at half of its first segment; N encoders did
	// more work than any single admitted one and delivered nothing, for two
	// minutes ("perpetual spinner").
	//
	// Riding over the cap for the grace window is safe: pauseSiblingsBehind /
	// pauseOtherVariants SIGSTOP every non-requested encoder, so the cap's CPU
	// intent holds - the shield only defers the KILL. Once a session has
	// delivered (its frontier moved past its start) or the window has passed,
	// the caps bite exactly as before. Default 60s: past the segment wait
	// timeout, so a slow first segment gets a second wait cycle on the same
	// encoder instead of a fresh spawn.
	NewbornEvictionShield time.Duration

	// MaxConcurrentVariantsPerHash bounds how many DISTINCT rungs (variantIdx) may
	// have a live encoder for one film at once - the anti-thrash ceiling on a weak
	// NAS where each rung is its own full 4K decode (the software path never shares
	// a decode across rungs). Past the cap the least-recently-used rung's encoder is
	// evicted (its segments stay on disk for the LRU disk evictor). Default 2; set
	// to 1 on the weakest boxes so a level switch fully releases the old rung before
	// the new one runs. Distinct from MaxSessionsPerHash, which bounds TOTAL encoders
	// (including several positions of ONE rung); this bounds distinct rungs.
	MaxConcurrentVariantsPerHash int

	// KillPausedAfter, when > 0, kills (not just SIGSTOPs) a session that has
	// been continuously paused for at least this long - reclaiming its ffmpeg
	// RSS (~1 GB) on a memory-constrained box. Disk segments are PRESERVED (no
	// CleanupHash): a resume replays them instantly from disk while a fresh
	// `-ss` ffmpeg cold-starts in the background, so the kill is invisible to
	// the player as long as ResumeCushionSegs already covers the gap. The idle
	// GC (InactivityTimeout) and the disk LRU evictor reclaim the segments
	// later on their own schedule. Default 0: disabled, matching the pre-feature
	// behaviour (a paused session only ever dies via sweepIdle).
	KillPausedAfter time.Duration

	// ResumeCushionSegs is the anti-latency guarantee gating KillPausedAfter: a
	// paused session is only actually killed once the segment cache already
	// holds this many segments beyond its frontier. Below the cushion the
	// session is SIGCONTed instead (it is paced fast - 1.5x/8x - so it builds
	// the missing cushion quickly) and revisited on a later sweep. Default 8.
	ResumeCushionSegs int

	// SeekReachWindowSegs is how many segments AHEAD of a session's production
	// frontier a request may be and still reuse that (paced) encoder rather than
	// seek a fresh one. Within the window the player waits a beat for the
	// sequential encoder to arrive; beyond it (a real forward seek) waiting would
	// blow the wait budget - a 4K HDR encoder paced at 1.5× takes minutes to cross
	// hundreds of segments - so a fresh encoder seeks (`-ss`) straight to the
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
	if out.NewbornEvictionShield <= 0 {
		out.NewbornEvictionShield = 60 * time.Second
	}
	if out.SeekReachWindowSegs <= 0 {
		out.SeekReachWindowSegs = 16
	}
	if out.ResumeCushionSegs <= 0 {
		out.ResumeCushionSegs = 8
	}
	// KillPausedAfter has no fallback - 0 means disabled, and stays disabled.
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
	// A session is only ever reused for a request of the SAME variantIdx - rungs
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
	// spawnedAt is when this encoder started. Together with "frontier still at
	// startSeg" it defines a NEWBORN - a session that has not yet delivered its
	// first segment and is therefore exactly what an active Wait depends on.
	// Newborns are shielded from cap eviction for NewbornEvictionShield (see
	// the option doc: killing them turns overload into the 2026-07-15
	// perpetual-spinner livelock). Guarded by mu (tests age it).
	spawnedAt time.Time
	paused    bool
	// pausedSince is when this session was most recently SIGSTOPped (zero when
	// running). Set by sweepPause/pauseEach, cleared by resumeIfPaused. Drives
	// the KillPausedAfter sweep: a session paused continuously past that
	// threshold is a kill candidate (see sweepKillPaused).
	pausedSince time.Time
	// useFMP4 mirrors the fingerprint's UseFMP4 at session start - needed by
	// sweepKillPaused to probe the segment cache (SegmentCache.IsReady takes
	// useFMP4 explicitly; the session has no other handle to the fingerprint).
	useFMP4   bool
	teardown  func()
	cancelCtx context.CancelFunc
	done      chan struct{}
	// metricDecOnce guarantees the HLSVODSessions gauge is decremented EXACTLY
	// once per session, whether it ends by natural ffmpeg EOF (the reap goroutine
	// runs, killSession does not) or by eviction/kill (killSession runs). Without
	// it a fully-watched software session - the nominal weak-NAS single-variant
	// case - would Inc but never Dec, leaking the gauge upward. Mirrors the
	// orchestrator's same-named guard.
	metricDecOnce sync.Once
	// videoSlotAcquired records whether startSession successfully took a
	// videoLimiter slot for THIS session (always false for a copy-mode
	// session, and false when videoLimiter is unwired). Read-only after
	// construction - safe without a lock.
	videoSlotAcquired bool
	// videoSlotReleaseOnce guarantees the videoLimiter slot this session holds
	// is released EXACTLY once, whichever of the two exit paths runs first: the
	// natural-EOF reap goroutine or an explicit killSession. Mirrors
	// metricDecOnce's same exactly-once contract for the metrics gauge.
	videoSlotReleaseOnce sync.Once
}

// releaseVideoSlot frees s's videoLimiter slot exactly once. No-op when the
// session never held one (copy mode, or videoLimiter unwired).
func (p *SoftwareProducer) releaseVideoSlot(s *swSession) {
	if !s.videoSlotAcquired || p.videoLimiter == nil {
		return
	}
	s.videoSlotReleaseOnce.Do(func() { p.videoLimiter.Release(s.hash) })
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
// at manifest-time and segment-time - the cache-hash invariant.
// SetCopyMode enables the video-COPY rung on this producer instance (the
// Paced remux: one -c:v copy session per live position, seek =
// restart). Call ONCE before first use - the wiring builds a DEDICATED copy
// instance for the remux path, distinct from the transcode instance the selector
// returns, so the two never share a manifest. nil planner = pure transcode.
//
// seekProber resolves where a pre-input -ss really lands (sparse container seek
// indexes land on the preceding cue point, possibly GOPs before the requested
// keyframe). nil = no landing correction: mid-stream copy sessions then assume
// they start at the requested keyframe, which is only safe for sources with a
// dense seek index (one cue per keyframe).
func (p *SoftwareProducer) SetCopyMode(planner *CopyPlanner, seekProber SeekLandingProber) {
	p.copyPlanner = planner
	p.seekProber = seekProber
}

// WithVideoSessionLimiter wires the shared cap on concurrently-ENCODING video
// sessions. Pass the SAME *VideoSessionLimiter instance given to the hardware
// SegmentOrchestrator so the two producers draw from one pool. Call before
// first use (not concurrency-safe with EnsureSegmentRange). nil (the zero
// value if never called) means unlimited - byte-identical to the pre-feature
// behaviour. Never call this on a copy-mode instance (SetCopyMode-wired): a
// copy session never encodes video, so gating it here would wrongly starve
// remux-only playback under the video encode cap.
func (p *SoftwareProducer) WithVideoSessionLimiter(limiter *VideoSessionLimiter) *SoftwareProducer {
	p.videoLimiter = limiter
	return p
}

// planIndexForLanding maps the demuxer's REAL seek landing (seconds) onto the
// copy plan: the index of the plan segment whose start keyframe the demuxer
// landed on. The landing is a keyframe and the plan has one boundary per
// keyframe, so it normally matches a boundary exactly (±100 ms tolerance for
// timestamp rounding); a non-matching landing falls back to the segment that
// CONTAINS it, which still bounds the timestamp error to under one GOP and is
// strictly better than assuming the requested target.
func planIndexForLanding(segs []CopySegment, landingSec float64) int {
	landingMs := int64(landingSec*1000 + 0.5)
	const tolMs = 100
	idx := 0
	for i, s := range segs {
		if s.StartMs <= landingMs+tolMs {
			idx = i
			continue
		}
		break
	}
	return idx
}

// copyReadRate returns the input-pacing rate for the session being started. The
// transcode flow paces at opts.ReadRate (default 1.5x) to keep a CPU-bound
// encoder near wall-clock. The copy flow paces LOOSELY at 8x: fast enough that
// the client buffer never starves and seeks land instantly (each ~10 s segment
// muxes in ~1.2 s, plus a 30 s full-speed initial burst - see
// appendCopyPacingBurst), but bounded so the ~free video copy cannot race
// minutes ahead of the CPU-bound audio re-encode inside ffmpeg. A fully UNPACED
// copy did exactly that on the prod NAS: video outran audio until the muxer's
// 10 s max_interleave_delta force-flushed, and segments carried audio up to
// ~8.3 s behind their video (the audible A/V desync). 8x also softens the
// burst-write pressure on the segment cache disk. Do NOT lower this to ~1.5x:
// that pinned the client to the production frontier (the old buffering-spinner
// regression).
func (p *SoftwareProducer) copyReadRate() float64 {
	if p.copyPlanner != nil {
		return 8
	}
	return p.opts.ReadRate
}

// Manifest serves the copy rendition's keyframe-aligned VOD media playlist
// (variable #EXTINF from the keyframe plan). Implements ManifestProvider.
//
// ⚠️ The type-assertion in serveVariant is STRUCTURAL - it matches *any*
// *SoftwareProducer, including the transcode instance (copyPlanner nil), NOT just
// a copy instance. So serveVariant MUST gate the ManifestProvider branch on
// isCopyRequest; without that gate the transcode path calls this and 500s (see
// the regression in commit history). This method returning an error when not in
// copy mode is only a backstop - the real guard is the handler's copy gate.
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
	// Copy mode = the paced remux: ONE rung at the source resolution with the
	// video copied (the handler advertises the source avc1.PPCCLL) and audio→AAC.
	// No transcode ladder - direct-play quality is the single rendition.
	if p.copyPlanner != nil {
		w, h := src.Width, src.Height
		if w <= 0 || h <= 0 {
			w, h = 1920, 1080 // safe default so the fingerprint stays deterministic
		}
		const copyDefaultBitrateKbps int64 = 5_000
		// "-kf4" suffix = the cache GENERATION of the copy plan/session contract
		// (every-keyframe plan + seek-landing-corrected sessions). The Name is
		// part of the cache fingerprint, so bumping it orphans cached copy
		// segments produced under an older, incompatible contract: "-kf3" plans
		// invented a segment for the leading partial GOP on sources whose first
		// keyframe is not at 0, so every seek-spawned .m4s they cached is filed
		// one index too high; "-kf2" seek segments carried audio content trimmed
		// at the -ss target but labeled from the landing (audio early by up to a
		// GOP), "-kf" ones a mislabeled video timeline, and the unsuffixed
		// generation ≥6s-coalesced numbering. Bump this whenever produced segment
		// content/labeling changes for the same fingerprint inputs.
		return []transcoding.HLSProfile{{
			Name:         fmt.Sprintf("%dp-copy-kf4", h),
			Width:        w,
			Height:       h,
			VideoBitrate: copyDefaultBitrateKbps,
			AudioBitrate: 192,
			Copy:         true, // folds into the cache fingerprint (copy vs transcode never alias)
		}}
	}

	sourceCapped := transcoding.CapLadderToSource(base, src.Width, src.Height)

	// Codec-aware software ceiling: AV1 (libsvtav1) is far heavier than H.264 and
	// carries its own SoftwareAV1MaxHeight; every other codec uses SoftwareMaxHeight.
	maxH := capacity.SoftwareCeilingFor(decision.VideoCodec)
	if needsHDRTonemap(decision) && capacity.SoftwareHDRMaxHeight > 0 && capacity.SoftwareHDRMaxHeight < maxH {
		maxH = capacity.SoftwareHDRMaxHeight
	}

	// The natural ladder capped to the sustainable ceiling - already ascending
	// with a 360p floor. No collapse: Auto needs the lower rungs to fall back to
	// and the viewer needs them to choose from. When the ceiling dropped rungs
	// the source could feed, the top surviving rung inherits the first dropped
	// rung's bitrate budget (see BoostCappedTopRung) - quality the box can
	// afford at the height it can sustain.
	capped := transcoding.CapLadderToHeight(sourceCapped, maxH)
	return transcoding.BoostCappedTopRung(sourceCapped, capped)
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
// NEVER returns ErrCapacityExceeded - the software model has no global session
// cap. A second media never sheds the first: both get their own long-lived
// encoder, paced so the host degrades gracefully under the input-rate cap
// rather than 503-looping.
//
// Multi-position per hash (the multi-tab coexistence guarantee):
//   - cache hit: no ffmpeg, just bump the covering session's lastAccess
//   - a live session covers the request (its startSeg <= segmentIdx): reuse the
//     closest-behind one; its sequential encoder is already producing toward it
//   - no covering session (e.g. a second viewer cold-starting BELOW a deep
//     viewer's start): spawn a new encoder ALONGSIDE at segmentIdx - never kill
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
		// the idle-pause loop suspended it - the client is pulling again). Commit
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

	// Cross-rung spawn guard: while at the per-hash cap with every sibling
	// HOT and ESTABLISHED (past the newborn shield - startup probes all rungs
	// at once and must keep coexisting, cf the 2026-07-15 spinner) and NONE
	// serving the requested rung, refuse the newcomer instead of letting the
	// cap evict a hot encoder (see ErrRungBusy). Same-rung newcomers
	// (multi-viewer, deep forward seeks) keep the spawn-alongside behaviour.
	p.mu.Lock()
	if len(p.sessions[hash]) >= p.opts.MaxSessionsPerHash {
		now := time.Now()
		allHotEstablished, sameRung := true, false
		for _, sib := range p.sessions[hash] {
			sib.mu.Lock()
			if p.newbornLocked(sib, now) || now.Sub(sib.lastAccess) > activeRungWindow {
				allHotEstablished = false
			}
			sib.mu.Unlock()
			if sib.variantIdx == variantIdx {
				sameRung = true
			}
		}
		if allHotEstablished && !sameRung {
			p.mu.Unlock()
			return ErrRungBusy
		}
	}
	p.mu.Unlock()

	// Nothing reaches this position in time: either no session covers it, or the
	// closest-behind one is a forward seek away (its frontier is too far below).
	// Start a fresh encoder seeking (`-ss`) to segmentIdx, ALONGSIDE any existing
	// sessions for the hash - we do NOT kill them. A second viewer cold-starting
	// below a deep viewer, or a viewer jumping forward past the live frontier, must
	// not yank a sibling another viewer consumes; they coexist, bounded by
	// MaxSessionsPerHash (LRU-evicted).
	return p.startSession(ctx, fp, variantIdx, segmentIdx)
}

// findCoveringSessionLocked returns the live session best placed to serve
// segmentIdx FOR THE GIVEN VARIANT: the same-variant session with the largest
// startSeg still <= segmentIdx (the closest-behind encoder, already producing
// forward toward it). Sessions producing a DIFFERENT rung are skipped - reusing
// one would never write this variant's segment file (the cache Wait polls
// per-variant), so the caller must spawn a fresh encoder for it. Returns nil when
// no same-variant session started at or below segmentIdx. Must hold p.mu.
func (p *SoftwareProducer) findCoveringSessionLocked(hash string, variantIdx, segmentIdx int) *swSession {
	var best *swSession
	bestStart := -1
	for _, s := range p.sessions[hash] {
		s.mu.Lock()
		// Skip corpses: killSession nulls cancelCtx first but only removes the
		// session from p.sessions after SIGTERM + process wait, so a dying encoder
		// stays matchable meanwhile. Reusing one leaves the caller waiting on a
		// segment nobody will write, and makes startSession's race check discard
		// the live replacement.
		dead := s.cancelCtx == nil
		sameVariant := s.variantIdx == variantIdx
		start := s.startSeg
		s.mu.Unlock()
		if dead {
			continue
		}
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

	srcPath, _, err := p.source.ResolveSource(ctx, fp)
	if err != nil {
		return fmt.Errorf("software: startSession: resolve source: %w", err)
	}

	// Copy-mode divergences (gated). nil planner → transcode, byte-identical to
	// before. Resolved up front, before any resource is acquired, so a missing
	// plan returns cleanly. The frontier/seek/reach math downstream is in segment
	// COUNT, so only StartSec (the -ss offset) differs for variable-length copy
	// segments: segment N seeks to its keyframe start, not N×SegmentSeconds.
	videoDecision := "transcode"
	startSec := float64(startSeg) * p.opts.SegmentSeconds
	seekTargetSec := 0.0
	var copyPlanOracle func(int) (int64, bool)
	if p.copyPlanner != nil {
		segs, planErr := p.copyPlanner.Plan(ctx, fp.MediaID)
		if planErr != nil {
			return fmt.Errorf("software: startSession: copy plan for %s: %w", fp.MediaID, planErr)
		}
		if startSeg < 0 || startSeg >= len(segs) {
			return fmt.Errorf("software: startSession: copy startSeg %d out of range (%d segments)", startSeg, len(segs))
		}
		videoDecision = "copy"
		// Plan oracle for the publish pump's runtime tripwire: every produced
		// segment's timeline is checked against its plan position, so any
		// future regression of the plan↔ffmpeg contract raises an ERROR log +
		// metric instead of an audible desync. segs is the planner's cached,
		// immutable plan - safe to capture.
		planSegs := segs
		copyPlanOracle = func(idx int) (int64, bool) {
			if idx < 0 || idx >= len(planSegs) {
				return 0, false
			}
			return planSegs[idx].StartMs, true
		}
		// The copy session runs `-hls_time 0` (split at EVERY keyframe - see
		// hlsTimeFor), the only split rule that matches the every-keyframe plan
		// for cold and seek sessions alike.
		//
		// Mid-stream start: a pre-input -ss seeks via the container's seek
		// index, which is SPARSE in real-world files (mkvmerge: one MKV cue per
		// ~5 s cluster) - ffmpeg lands on the cue point AT-OR-BEFORE the target,
		// possibly whole GOPs before the requested keyframe. The session's plan
		// bookkeeping (StartSec, -start_number, the tfdt rebase) must follow the
		// LANDED keyframe, not the requested one, or every video timestamp it
		// emits is shifted by the gap while the re-encoded audio (trimmed at the
		// exact -ss time) stays true - the measured post-deep-seek A/V desync.
		// Producing a few extra segments before the requested one is the cheap,
		// correct outcome: they are exact plan segments (cache-consistent).
		if startSeg > 0 {
			// seekTargetSec is the ONE -ss the session will emit AND the exact
			// timestamp the landing probe runs against: probe(T) == landing(T)
			// by construction, so the bookkeeping below stays true. (Re-deriving
			// -ss from the landed keyframe would land one keyframe earlier again
			// - the ffmpeg CLI's ~130 ms DTS-heuristic seek offset.)
			seekTargetSec = float64(segs[startSeg].StartMs)/1000.0 + transcoding.CopySeekNudgeSec
			if p.seekProber != nil {
				requested := startSeg
				landing, probeErr := p.seekProber.ProbeSeekLanding(ctx, srcPath, seekTargetSec)
				switch {
				case probeErr != nil:
					p.logger.Warn("software: copy seek-landing probe failed, assuming exact landing",
						slog.Float64("target", seekTargetSec), slog.Any("err", probeErr))
				default:
					if j := planIndexForLanding(segs, landing); j < startSeg {
						p.logger.Info("software: copy seek landed before the requested keyframe (sparse seek index / dts heuristic)",
							slog.Int("requested_seg", requested),
							slog.Int("landed_seg", j),
							slog.Float64("landing_sec", landing))
						startSeg = j
					}
				}
			}
		}
		startSec = float64(segs[startSeg].StartMs) / 1000.0
	}

	// Transcode-seek tripwire: a mid-stream transcode session (no copy plan,
	// startSeg > 0) re-encodes fixed-length segments, so the rebase pump can be
	// checked against a linear oracle - segment idx must land at idx×segLen.
	// Cold transcode (startSeg == 0) is 0-based by construction and needs no
	// check. Copy sessions already have their plan oracle above.
	timelineDriftKind := "copy"
	if videoDecision == "transcode" && startSeg > 0 {
		segLenMs := int64(math.Round(p.opts.SegmentSeconds * 1000))
		copyPlanOracle = func(idx int) (int64, bool) {
			if idx < 0 {
				return 0, false
			}
			return int64(idx) * segLenMs, true
		}
		timelineDriftKind = "transcode_seek"
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
		AudioDecision:      fp.AudioDecision(),
		AudioTrackIndex:    fp.AudioTrack,
		AudioUserOffsetSec: float64(fp.AudioOffsetMs) / 1000.0,
		SubtitleTrackIndex: fp.SubtitleBurnInTrack,
		SubtitleCodec:      fp.SubtitleBurnInCodec,
		HDRSupported:       false, // tonemap is part of the decision; HDRSupported=false forces it
		ToneMap:            fp.HDRTonemap,
		AudioNormalize:     fp.AudioNormalize,
		StartSec:           startSec,
		// Absolute segment index this session starts at. For copy mode the
		// builder uses THIS (not StartSec/segLen) as ffmpeg's -start_number,
		// because keyframe-aligned copy segments are variable-length so the
		// time→index map is non-linear; a forward seek to plan-segment K must
		// emit stream_N_<K>.m4s, not stream_N_<floor(StartSec/segLen)>.m4s.
		StartNumber: startSeg,
		// Copy rung copies on seek too (plan StartSec is a keyframe boundary), so
		// a mid-stream session keeps the SOURCE avcC instead of emitting a libx264
		// init that would poison the shared init. No-op when not in copy mode.
		CopyOnSeek: p.copyPlanner != nil,
		// The exact -ss the probe validated; bookkeeping (StartSec/StartNumber)
		// reflects its real landing. 0 on cold starts.
		SeekTargetSec: seekTargetSec,
		// Plan oracle for the publish pump's timeline tripwire: the copy plan
		// (copy mode) or the linear transcode-seek oracle set just above; nil
		// for cold transcode.
		ExpectedSegmentStartMs: copyPlanOracle,
		TimelineDriftKind:      timelineDriftKind,
		// Input pacing: the defining software-flow knob. Keeps the single
		// sequential encoder near wall-clock so it never races ahead of the
		// viewer (no abort-and-probe client → no need to burst).
		//
		// COPY MODE: paced LOOSELY at 8x + a 30 s initial burst instead of the
		// transcode 1.5x - fast enough that the client never starves and seeks are
		// instant, bounded so video copy can't outrun the audio re-encode into the
		// muxer's interleave force-flush (the prod A/V desync). See copyReadRate.
		ReadRate:         p.copyReadRate(),
		VideoCodec:       fp.VideoCodec,
		SourceVideoCodec: fp.SourceVideoCodec,
		DeinterlaceAll:   fp.DeinterlaceAll,
		SourceSAR:        fp.SourceSAR,
		AudioCodec:       fp.AudioCodec,
		MaxAudioChannels: fp.MaxAudioChannels,
		ForceSidecarSubs: fp.ForceSidecarSubs,
		UseFMP4:          fp.UseFMP4,
		// HWAccel deliberately left empty - this is the SOFTWARE producer. Even
		// if the fingerprint carried a HW backend (it won't, the selector only
		// routes here when HW is unusable), we never run the GPU pipeline here.
	}

	// Video-encode capacity gate: only the TRANSCODE instance (copyPlanner ==
	// nil) burns a full CPU core encoding video, so only it draws from the
	// shared limiter. A copy (remux) instance's `-c:v copy` costs negligible
	// CPU and must never be gated here - copyPlanner != nil skips this whole
	// block. Acquired BEFORE ffmpeg spawns (the actual CPU-consuming action) and
	// released on every exit path below exactly once (see videoSlotAcquired /
	// releaseVideoSlot).
	videoSlotAcquired := false
	if p.copyPlanner == nil && p.videoLimiter != nil {
		if !p.videoLimiter.TryAcquireFor(hash, LadderClassOf(fp), topProfileHeight(procOpts.Profiles), SourceClassOf(fp)) {
			teardown()
			return ErrVideoCapacityExhausted
		}
		videoSlotAcquired = true
	}

	sessionCtx, cancel := context.WithCancel(p.shutdownCtx)
	handle, err := p.process.Start(sessionCtx, procOpts)
	if err != nil {
		cancel()
		teardown()
		if videoSlotAcquired {
			p.videoLimiter.Release(hash)
		}
		return fmt.Errorf("software: startSession: ffmpeg start: %w", err)
	}

	session := &swSession{
		hash:              hash,
		mediaID:           fp.MediaID,
		handle:            handle,
		variantIdx:        variantIdx,
		videoSlotAcquired: videoSlotAcquired,
		startSeg:          startSeg,
		frontier:          startSeg,
		lastAccess:        time.Now(),
		spawnedAt:         time.Now(),
		useFMP4:           fp.UseFMP4,
		teardown:          teardown,
		cancelCtx:         cancel,
		done:              make(chan struct{}),
	}

	p.mu.Lock()
	// Race: another goroutine may have started a session reaching startSeg between
	// our coveringWithinReach decision and now. Re-check under the lock with the
	// SAME reach-aware test; if a sibling now reaches this exact position, discard
	// the loser (avoid a redundant duplicate encoder for the same offset). A
	// distant sibling that merely started BELOW startSeg does not count - that is
	// the forward-seek case we are spawning this fresh encoder for; distinct live
	// positions are the coexistence we want, not a race.
	if winner := p.coveringWithinReachLocked(hash, variantIdx, startSeg); winner != nil {
		p.mu.Unlock()
		cancel()
		if stopErr := handle.Stop(); stopErr != nil {
			p.logger.Debug("software: stop on start race lost (ignored)", slog.Any("err", stopErr))
		}
		teardown()
		// This session never reaches the reap goroutine or killSession (it was
		// discarded before either could take ownership), so release its video
		// slot here - the only place that will ever do so for it.
		p.releaseVideoSlot(session)
		return nil
	}
	p.sessions[hash] = append(p.sessions[hash], session)
	p.spawnedHashes[hash] = struct{}{}
	// Count the session live BEFORE anything can reap it. The reap goroutine
	// below fires metricDecOnce as soon as ffmpeg exits, and an encoder that dies
	// instantly (codec refused, source gone) would consume the Once before this
	// Inc ran - stranding the gauge one above zero forever.
	metrics.HLSVODSessions.Inc()
	// Enforce the per-hash cap: if we just crossed MaxSessionsPerHash, evict the
	// least-recently-used sibling (never the one we just appended). Kill it
	// outside the lock - killSession reacquires p.mu.
	victim := p.evictLRUIfFullLocked(hash)
	variantVictims := p.evictExcessVariantsLocked(hash, variantIdx)
	liveCount := len(p.sessions[hash])
	overCap := liveCount > p.opts.MaxSessionsPerHash
	p.mu.Unlock()

	if victim != nil {
		p.logger.Info("software: per-hash cap reached, evicting LRU encoder",
			slog.String("hash", hash), slog.Int("cap", p.opts.MaxSessionsPerHash))
		p.killSession(victim, "lru_cap")
	} else if overCap {
		// Every older sibling is a newborn an active Wait may depend on:
		// keeping them (paused) over the cap for the grace window is what
		// prevents the 2026-07-15 kill/respawn livelock. Logged because an
		// operator watching the encoder count deserves to know the cap is
		// deliberately, temporarily exceeded.
		p.logger.Info("software: per-hash cap exceeded, newborn encoders kept until first delivery",
			slog.String("hash", hash), slog.Int("cap", p.opts.MaxSessionsPerHash),
			slog.Int("live", liveCount))
	}
	for _, v := range variantVictims {
		p.logger.Info("software: distinct-variant cap reached, evicting LRU rung's encoder",
			slog.String("hash", hash), slog.Int("cap", p.opts.MaxConcurrentVariantsPerHash))
		p.killSession(v, "variant_cap")
	}

	// On a weak NAS only one heavy software transcode fits. This encoder just
	// started AHEAD of the viewer's previous position (a forward seek), so hand it
	// the CPU at once: SIGSTOP the hash's encoders strictly BEHIND it instead of
	// letting the abandoned one contend for up to PauseIdleThreshold seconds - a
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
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(sessionCtx, "software_producer: detached reap goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
			}
		}()
		naturalExit := false
		select {
		case <-handle.Done():
			naturalExit = true
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
		// Natural EOF path: killSession never runs, so the gauge must be
		// decremented here. Once-guarded so a concurrent kill doesn't double-Dec.
		// Count natural completions (mutually exclusive with killSession's
		// HLSVODSessionsKilled) so spawns − (completed + killed) reconciles the
		// gauge and surfaces any teardown leak.
		session.metricDecOnce.Do(func() {
			metrics.HLSVODSessions.Dec()
			if naturalExit {
				metrics.HLSVODSessionsCompleted.Inc()
			}
		})
		// Natural exit: killSession never ran for this session, so free its video
		// slot here. Once-guarded (see releaseVideoSlot) so a concurrent kill that
		// raced this reap can't double-release.
		p.releaseVideoSlot(session)
	}()

	p.logger.Info("software: session started",
		slog.String("hash", hash),
		slog.Int("start_seg", startSeg),
		// The EFFECTIVE pacing of this session (copy mode overrides the
		// configured transcode rate) - logging p.opts.ReadRate here once hid
		// that copy sessions ran unpaced.
		slog.Float64("read_rate", procOpts.ReadRate),
		slog.String("source", srcPath))
	metrics.HLSVODRafaleSpawned.WithLabelValues("software").Inc()
	return nil
}

// PreWarm starts the one software ffmpeg for fp in the background (cold-start
// optimisation: kick the slow software decode off before the player asks for
// segment 0). Bounded by shutdownCtx so Close cancels in-flight pre-warms.
func (p *SoftwareProducer) PreWarm(fp DecisionFingerprint, variantIdx, startSegIdx int) {
	if p.shutdownCtx.Err() != nil {
		return
	}
	p.prewarmWG.Go(func() {
		// Re-check inside the goroutine: Close cancels shutdownCtx BEFORE
		// prewarmWG.Wait(), so a Go that registered after Wait returned must bail
		// before EnsureSegmentRange re-creates a hash dir Close just wiped.
		if p.shutdownCtx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(p.shutdownCtx, 30*time.Second)
		defer cancel()
		if err := p.EnsureSegmentRange(ctx, fp, variantIdx, startSegIdx); err != nil {
			p.logger.Debug("software: prewarm failed (non-fatal)",
				slog.String("media_id", fp.MediaID), slog.Any("err", err))
		}
	})
}

// ErrRungBusy refuses a cross-rung spawn while every live sibling encoder of
// the hash is actively serving another rung. An ABR up-switch probe used to
// spawn a second encoder, hit the per-hash cap and evict the rung the viewer
// was WATCHING (lru_cap kill/respawn every ~20s, prod 2026-07-29 on a 4K
// source). A weak host cannot encode two rungs of one title at once: the
// probe gets a FAST 503 and the player stays on its rung.
var ErrRungBusy = errors.New("software: another rung of this title is actively encoding")

// activeRungWindow is how recently a sibling must have been accessed to count
// as "actively serving". Comfortably above the segment cadence (a watched
// rung is touched every few seconds); an abandoned rung falls idle within it,
// so a DELIBERATE quality switch spawns normally after one window.
const activeRungWindow = 10 * time.Second

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
		// advance its frontier - keeps the seek-reach test (coveringWithinReach)
		// honest about how far this session has actually reached.
		s.frontier = segmentIdx
	}
	s.mu.Unlock()
	p.resumeIfPaused(s)
}

// coveringWithinReachLocked returns the live session that can serve segmentIdx
// within the wait budget: the closest-behind encoder (largest startSeg <=
// segmentIdx) whose production frontier is at most SeekReachWindowSegs behind the
// request. Returns nil when the best candidate is further behind - that's a
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

// Compile-time assertion: SoftwareProducer satisfies the StreamProducer seam.
var _ StreamProducer = (*SoftwareProducer)(nil)
