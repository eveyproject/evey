package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// ErrCapacityExceeded is returned by EnsureSegmentRange when the orchestrator is
// already running MaxActiveSessions ffmpeg rafales and refuses to spawn another.
// The HTTP handler maps it to 503 + Retry-After so the player backs off and
// retries once a slot frees, rather than letting an authenticated client melt a
// weak host's CPU by fanning out across many fingerprints (distinct media, or a
// single media diversified via audio/sub/hdr query params). This is the global
// DoS backstop layered on top of the per-hash MaxRafalesPerHash cap.
var ErrCapacityExceeded = errors.New("transcoding: max active sessions reached")

// SegmentOrchestrator owns the ffmpeg sessions that produce segments into the
// SegmentCache. Phase 1: one active session per DecisionHash; on a cache miss
// the orchestrator spawns ffmpeg starting at the requested segment, lets it run
// LookAheadSegments segments forward, then lets it exit naturally.
//
// Concurrency: thread-safe. EnsureSegmentRange may be called from any number
// of goroutines; only one ffmpeg per DecisionHash is allowed to run at a time.
//
// Cancel-on-seek (audit indies: hls-media-server pattern): if a request arrives
// for a segment outside the active session's [startSeg, endSeg] window, the
// session is killed and a fresh one is spawned at the new offset. Avoids paying
// for ahead-encoded segments the user just seeked past.
type SegmentOrchestrator struct {
	cache   *SegmentCache
	process transcoding.HLSProcess
	source  SourceResolver
	opts    OrchestratorOptions
	logger  *slog.Logger

	// hwDemote is the level-2 safety-net sink: invoked (off the reap goroutine)
	// when a session's ffmpeg exits with a hardware-capability failure, so the
	// calibration service can demote the host capability. Optional/nil-safe -
	// when unset, a HW failure just falls through to the normal 503-retry.
	hwDemote func(ctx context.Context, report HWFailureReport)

	// hwRouteBlocks is the per-title half of the fallback ladder: a title whose
	// GPU session failed for a deterministic reason is served from the CPU at
	// its next request (the resolver reads HWRouteBlocked), before and
	// regardless of what the host-level proof in hwDemote concludes.
	hwRouteBlocks *HWRouteBlocklist

	// shutdownCtx is the long-lived parent context for every ffmpeg session
	// the orchestrator spawns. Never the HTTP request context - that one is
	// cancelled the moment the segment is served, which would tear down the
	// rafale before the next segment is requested. Cancelled only by Close.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	mu sync.Mutex
	// sessions maps a fingerprint hash to the rafales currently active
	// for it. Multiple concurrent rafales per hash exist when several
	// clients watch the same media at different positions (two browser
	// tabs, multi-user replay of the same film). Each rafale serves a
	// disjoint segment range; ffmpeg writes its own `stream_N_<seg>.m4s`
	// files so the per-variant filename never collides between rafales.
	//
	// Bounded by maxRafalesPerHash to keep CPU under control on
	// pathological hosts - beyond the cap the LRU rafale is evicted.
	sessions map[string][]*orchestratorSession

	// pendingSpawns holds the spawn windows that have been claimed but not yet
	// registered in `sessions`. It is what makes a burst of concurrent requests
	// for the same position exec ONE ffmpeg instead of N (see
	// EnsureSegmentRange). Guarded by mu; entries live only for the duration of
	// a spawnSession call.
	pendingSpawns map[string][]*pendingSpawn

	// pendingSeeks holds, per hash, the LAST unserved-seek target whose spawn is
	// being debounced by opts.SeekCoalesceWindow. A burst of rapid seeks on one
	// fingerprint updates the single entry (last target wins) and re-arms its
	// timer instead of each seek spawning-then-LRU-evicting an encoder before it
	// produces. Guarded by mu; nil/empty when SeekCoalesceWindow is 0 (off) or no
	// seek is pending. Entries fire (spawn once) then delete via fireCoalescedSeek.
	pendingSeeks map[string]*pendingSeek

	stopGC chan struct{}
	gcDone chan struct{}

	stopThrottle chan struct{}
	throttleDone chan struct{}

	// closeOnce guards Close so a double shutdown (e.g. defer + explicit
	// Close, or two lifecycle owners) doesn't panic on a re-close of stopGC.
	closeOnce sync.Once

	// prewarmWG tracks PreWarm goroutines so Close() can drain them before
	// nuking the cache. Without this, a goroutine spawned by serveVariant
	// could write fresh files into the cache dir AFTER Close() ran its
	// cleanup pass - leaking tempdirs in tests and racing with the
	// orchestrator's lifecycle in prod.
	prewarmWG sync.WaitGroup

	// spawnedHashes is the set of every cache hash this orchestrator
	// has ever spawned a session for. We track it separately from the
	// live `sessions` map so Close() can cleanup the disk even when
	// sweepIdle has already deleted the session struct from the live
	// map - the disk segments stay on the host until an explicit
	// shutdown cleanup, and we don't want a goroutine ordering race
	// between sweepIdle and Close to leak them.
	//
	// KNOWN BOUNDED GROWTH: unlike the software producer (whose sweepIdle
	// reclaims the hash dir and can therefore also drop the set entry), the
	// orchestrator's disk is reclaimed by the cache's LRU eviction, which this
	// struct never observes - so entries here outlive their dir and accumulate
	// (~60 B per distinct hash) until Close. The Close-time CleanupHash on an
	// already-evicted hash is a harmless no-op, and the OOM horizon is years;
	// pruning would need an eviction callback from SegmentCache, not worth the
	// plumbing at this magnitude.
	spawnedHashes map[string]struct{}

	// sessionSem is a counting semaphore bounding the total number of live
	// ffmpeg rafales to opts.MaxActiveSessions. nil when uncapped
	// (MaxActiveSessions <= 0). A slot is acquired in spawnSession before ffmpeg
	// starts and released exactly once - by the reap goroutine when the session
	// exits, or by an early-return / spawn-race-lost path before the reap
	// goroutine takes ownership.
	sessionSem chan struct{}

	// videoLimiter caps the number of concurrently-ENCODING video sessions this
	// orchestrator may run, shared with the software producer's transcode
	// instance via the same *VideoSessionLimiter (every session this
	// orchestrator spawns encodes video - there is no copy/remux mode here).
	// nil = unwired = unlimited (byte-identical to the pre-feature behaviour).
	// Set via WithVideoSessionLimiter before first use.
	videoLimiter *VideoSessionLimiter
}

type orchestratorSession struct {
	mu         sync.Mutex
	hash       string
	mediaID    string // for RemoveSession reverse lookup (a single mediaID can have several hashes - different audio/sub/codec decisions)
	handle     transcoding.HLSProcessHandle
	startSeg   int // first segment this rafale is producing
	endSeg     int // last segment (inclusive) the rafale will produce
	lastAccess time.Time
	teardown   func() // SegmentCache.RegisterSession's teardown
	cancelCtx  context.CancelFunc
	done       chan struct{} // closed when ffmpeg exits (success or error)
	// killed is set by killSession, under mu, BEFORE it stops ffmpeg: the reap
	// goroutine reads it to tell our own teardown from a natural exit. The
	// select on handle.Done()/sessionCtx.Done() cannot - Stop() closes the
	// handle before cancel() fires, so a watchdog kill read as "ffmpeg exited
	// on its own, no segment produced" and blocked the title's GPU route.
	killed bool

	// variantIdx is meaningful only when opts.SingleVariant is true. In that
	// mode each session encodes exactly one ABR rung and the session-match
	// logic in reachableSessionLocked uses it to keep per-rung sessions
	// isolated. When SingleVariant is false this field is 0 and unused.
	variantIdx int

	// Throttle state. clientHighWater is the largest segment index any client
	// has asked us to ensure for this session - used as a proxy for "how far
	// the client has consumed". paused tracks whether the ffmpeg process is
	// currently SIGSTOPped; useFMP4 is captured at spawn so the throttle loop
	// can query the cache without the fingerprint.
	clientHighWater int
	// clientRead is set the first time a CLIENT request lands on this rafale
	// (EnsureSegmentRange finds it covering, or touchSession hits its cache).
	// It separates a rafale someone is draining from a speculative one - a
	// PreWarm bet on a position the player never went to - so the GC can retire
	// the second on a much shorter fuse. serveVariant pre-warms segment 0
	// unconditionally, so every resume into a transcoded title opens a rafale
	// at 0 that nobody will ever read; on the ordinary InactivityTimeout it sat
	// there encoding a full ABR ladder ALONGSIDE the real one for the best part
	// of a minute (prod, 2026-09-08: rafale at seg 0 spawned 23:06:18, the real
	// one at seg 121 spawned 23:06:23, the useless one killed only at 23:07:19).
	clientRead bool
	// producedHighWater is the last segment index the throttle observed on
	// disk for this rafale, or startSeg-1 before anything lands. It is a
	// resume point for SegmentCache.HighestReadyFrom, which is why the
	// throttle no longer lists the whole hash directory every tick: ffmpeg
	// only ever appends, so each tick stats forward from here.
	producedHighWater int
	paused            bool
	useFMP4           bool
	// lastResumeAt rate-limits NotifyClientHint's SIGCONT path.
	// A malicious / buggy client could forge `bs=1` at 1000/sec and
	// keep us hammering the kernel with SIGCONT syscalls - idempotent
	// on a running process but wasted scheduler time. One Resume per
	// second per session is plenty (the player rebuffers more slowly
	// than that anyway).
	lastResumeAt time.Time

	// metricDecOnce guarantees the HLSVODSessions gauge is decremented EXACTLY
	// once per spawned session, whether it leaves via the reap goroutine (natural
	// ffmpeg exit - the dominant path when a viewer watches to the end) or via
	// killSession (GC / seek-cancel / watchdog). Without it the reap path never
	// decremented and the gauge climbed monotonically.
	metricDecOnce sync.Once

	// videoSlotAcquired records whether spawnSession successfully took a
	// videoLimiter slot for THIS session (false when videoLimiter is unwired).
	// Read-only after construction - safe without a lock.
	videoSlotAcquired bool
	// videoSlotReleaseOnce guarantees the videoLimiter slot this session holds
	// is released EXACTLY once, whichever of the two exit paths runs first: the
	// natural-EOF reap goroutine or an explicit killSession. Mirrors
	// metricDecOnce's same exactly-once contract for the metrics gauge.
	videoSlotReleaseOnce sync.Once
}

// releaseVideoSlot frees s's videoLimiter slot exactly once. No-op when the
// session never held one (videoLimiter unwired).
func (o *SegmentOrchestrator) releaseVideoSlot(s *orchestratorSession) {
	if !s.videoSlotAcquired || o.videoLimiter == nil {
		return
	}
	s.videoSlotReleaseOnce.Do(func() { o.videoLimiter.Release(s.hash) })
}

// NewSegmentOrchestrator wires the cache and the ffmpeg adapter together. The
// returned orchestrator owns a goroutine that sweeps idle sessions; call Close
// on shutdown to stop it cleanly.
func NewSegmentOrchestrator(
	cache *SegmentCache,
	process transcoding.HLSProcess,
	source SourceResolver,
	opts OrchestratorOptions,
	logger *slog.Logger,
) *SegmentOrchestrator {
	// Defensive: tests historically passed nil here; production wiring uses
	// slog.Default(). Fall back so a nil arg doesn't crash at startup.
	if logger == nil {
		logger = slog.Default()
	}
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	o := &SegmentOrchestrator{
		cache:          cache,
		process:        process,
		source:         source,
		opts:           opts.withDefaults(),
		logger:         logger.With("component", "segment_orchestrator"),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		sessions:       make(map[string][]*orchestratorSession),
		spawnedHashes:  make(map[string]struct{}),
		pendingSeeks:   make(map[string]*pendingSeek),
		stopGC:         make(chan struct{}),
		gcDone:         make(chan struct{}),
		stopThrottle:   make(chan struct{}),
		throttleDone:   make(chan struct{}),
	}
	if o.opts.MaxActiveSessions > 0 {
		o.sessionSem = make(chan struct{}, o.opts.MaxActiveSessions)
	}
	go o.gcLoop()
	if o.opts.ThrottleEnabled {
		go o.throttleLoop()
	} else {
		close(o.throttleDone)
	}
	return o
}

// SetHWDemoteSink wires the level-2 HW safety net: the sink is called when a
// session's ffmpeg exits with a hardware-capability failure (classified via the
// handle's Failure()), so the capability can be demoted for subsequent requests.
// Optional - left unset in tests / when no calibration service is present.
// WithVideoSessionLimiter wires the shared cap on concurrently-ENCODING video
// sessions. Pass the SAME *VideoSessionLimiter instance given to the software
// producer's transcode instance so the two draw from one pool - every session
// this orchestrator spawns encodes video (there is no copy/remux mode here),
// so it is unconditionally gated. Call before first use (not
// concurrency-safe with EnsureSegmentRange). nil (the zero value if never
// called) means unlimited - byte-identical to the pre-feature behaviour.
func (o *SegmentOrchestrator) WithVideoSessionLimiter(limiter *VideoSessionLimiter) *SegmentOrchestrator {
	o.videoLimiter = limiter
	return o
}

// SetHWFailureSink wires the level-2 safety net: every failed hardware session
// reports its class and its exact route here (ADR-0030, 7).
func (o *SegmentOrchestrator) SetHWFailureSink(sink func(ctx context.Context, report HWFailureReport)) {
	o.hwDemote = sink
}

// SetHWDemoteSink is the (class, codec) form of SetHWFailureSink, kept for the
// callers that do not read the route.
func (o *SegmentOrchestrator) SetHWDemoteSink(sink func(ctx context.Context, class transcoding.FailureClass, codec string)) {
	if sink == nil {
		o.hwDemote = nil
		return
	}
	o.hwDemote = func(ctx context.Context, r HWFailureReport) { sink(ctx, r.Class, r.Codec) }
}

// hwFailure builds the report of a failed hardware session from its fingerprint.
func hwFailure(class transcoding.FailureClass, fp DecisionFingerprint) HWFailureReport {
	return HWFailureReport{Class: class, Codec: fp.VideoCodec, Route: RouteCostKeyOf(fp)}
}

// WithHWRouteBlocklist wires the per-title GPU route block (chainable).
func (o *SegmentOrchestrator) WithHWRouteBlocklist(b *HWRouteBlocklist) *SegmentOrchestrator {
	o.hwRouteBlocks = b
	return o
}

// HWRouteBlocked reports whether mediaID's GPU route is blocked after a
// failure; the plan resolver's HWRouteBlocked hook. Nil-safe.
func (o *SegmentOrchestrator) HWRouteBlocked(mediaID string) bool {
	return o.hwRouteBlocks.Blocked(mediaID)
}

// ClearHWRouteBlocks forgets every per-title block: a fresh calibration (or a
// confirmed demotion) re-proves the routes, so what the old proof blocked is
// re-tried on the new one.
func (o *SegmentOrchestrator) ClearHWRouteBlocks() {
	o.hwRouteBlocks.Clear()
}

// HWRouteBlocks lists the live per-title blocks for the admin view.
func (o *SegmentOrchestrator) HWRouteBlocks() []HWRouteBlockEntry {
	return o.hwRouteBlocks.Entries()
}

// blockTitleRoute records that mediaID's GPU session failed: the title's next
// request resolves to the CPU route. Logged at Warn: it is the line that
// explains why a title plays on the CPU on a GPU host.
func (o *SegmentOrchestrator) blockTitleRoute(mediaID, hash string, accel transcoding.HWAccelType, reason string) {
	if o.hwRouteBlocks == nil || mediaID == "" {
		return
	}
	o.hwRouteBlocks.Block(mediaID, reason)
	o.logger.Warn("orchestrator: GPU session failed, title falls back to the CPU route",
		slog.String("media_id", mediaID),
		slog.String("hash", hash),
		slog.String("hw_backend", string(accel)),
		slog.String("reason", reason))
}

// productionPointLocked: the last segment known produced, startSeg-1 before any. Caller holds s.mu.
func productionPointLocked(s *orchestratorSession) int {
	return max(s.producedHighWater, s.startSeg-1)
}

// reachableSessionLocked returns the live session that covers segmentIdx
// (window or near-ahead band) within SeekReachWindowSegs of its production
// point. When every cover is further behind it returns the closest as `far`
// (fallback when no rafale can be spawned) with its point. Caller holds o.mu.
// SingleVariant restricts the match to the requested rung.
func (o *SegmentOrchestrator) reachableSessionLocked(hash string, variantIdx, segmentIdx int) (reachable, far *orchestratorSession, farPoint int) {
	farPoint = -1
	for _, s := range o.sessions[hash] {
		if o.opts.SingleVariant && s.variantIdx != variantIdx {
			continue
		}
		s.mu.Lock()
		// A dying rafale (cancelCtx nil) stays listed until ffmpeg exits; matching
		// it promises a segment nobody will produce.
		dead := s.cancelCtx == nil
		inWindow := segmentIdx >= s.startSeg && segmentIdx <= s.endSeg
		nearAhead := segmentIdx > s.endSeg && segmentIdx <= s.endSeg+o.opts.LookAheadSegments
		point := productionPointLocked(s)
		s.mu.Unlock()
		if dead || (!inWindow && !nearAhead) {
			continue
		}
		if segmentIdx-point <= o.opts.SeekReachWindowSegs {
			return s, nil, point
		}
		if far == nil || point > farPoint {
			far, farPoint = s, point
		}
	}
	return nil, far, farPoint
}

// PreWarm asks the orchestrator to start producing segments for fp in a
// background goroutine. Fire-and-forget: the spawned work outlives the
// HTTP request that triggered it but is bounded by the orchestrator's own
// shutdownCtx, so Close() cancels every in-flight pre-warm (clean
// teardown, no leaked tempdirs in tests).
//
// Used by the HLS-VOD handler on `serveVariant`: when the player fetches
// the variant playlist we kick off the ffmpeg rafale immediately, in
// parallel with the player parsing the playlist. Saves a 200 ms RTT in
// the cold-start path AND gives ffmpeg an extra head start when its first
// segment is slow (4K HEVC software decode).
func (o *SegmentOrchestrator) PreWarm(fp DecisionFingerprint, variantIdx, startSegIdx int) {
	// Skip cleanly if Close already ran. shutdownCtx.Err() is non-nil after
	// shutdownCancel - spawning a goroutine then would race the cache wipe.
	if o.shutdownCtx.Err() != nil {
		return
	}
	o.prewarmWG.Go(func() {
		// Re-check inside the goroutine: Close runs shutdownCancel() BEFORE
		// prewarmWG.Wait(), so if this Go registered after Wait already returned
		// (the outer check raced the cancel), shutdownCtx is already cancelled
		// here and we must bail before EnsureSegmentRange re-creates a hash dir
		// that Close just wiped.
		if o.shutdownCtx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(o.shutdownCtx, 30*time.Second)
		defer cancel()
		// Pre-warm targets the caller's requested variantIdx. In single-variant
		// mode that's the ONLY rung warmed - warming rung 0 unconditionally would
		// spawn a whole ffmpeg the player never asked for. In multi-variant mode
		// variantIdx is forwarded but ignored for session-matching (same as
		// EnsureSegmentRange everywhere else), so this is byte-for-byte identical
		// to before.
		if err := o.EnsureSegmentRange(ctx, fp, variantIdx, startSegIdx); err != nil {
			o.logger.Debug("orchestrator: prewarm failed (non-fatal)",
				slog.String("media_id", fp.MediaID),
				slog.Any("err", err))
		}
	})
}

// EnsureSegmentRange guarantees that segment `segmentIdx` of `variantIdx` will
// be produced (or is already on disk). Returns immediately - callers must use
// cache.Wait to block on segment readiness. The returned error is non-nil only
// on terminal setup failure (resolver/source unavailable, ffmpeg spawn refused).
//
// variantIdx identifies the ABR rung the client is requesting. When
// SingleVariant is OFF (the default) this value is forwarded to the cache
// fast-path check but otherwise ignored for session-matching - the existing
// multi-variant behaviour is preserved byte-for-byte. When SingleVariant is ON,
// each (hash, variantIdx) pair gets its own ffmpeg rafale.
//
// The orchestrator may spawn a fresh ffmpeg session, kill+respawn an existing
// one (cancel-on-seek), or short-circuit when the segment is already covered
// by a live session or is already in the cache.
func (o *SegmentOrchestrator) EnsureSegmentRange(
	ctx context.Context,
	fp DecisionFingerprint,
	variantIdx int,
	segmentIdx int,
) error {
	if segmentIdx < 0 {
		return fmt.Errorf("EnsureSegmentRange: negative segmentIdx %d", segmentIdx)
	}
	hash := fp.Hash()

	// Fast path: segment already on disk (cache hit, no ffmpeg needed).
	// Use the caller's variantIdx so a per-rung check is accurate in both
	// single-variant and multi-variant modes.
	if o.cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: variantIdx, SegmentIdx: segmentIdx}, fp.UseFMP4) {
		o.touchSession(hash, variantIdx, segmentIdx)
		metrics.HLSVODCacheHits.Inc()
		return nil
	}
	metrics.HLSVODCacheMisses.Inc()

	// Multi-rafale per hash: walk every active rafale for this
	// fingerprint and reuse the first one whose [startSeg, endSeg]
	// window (or nearAhead extension) covers segmentIdx. If none
	// does, spawn a fresh rafale ALONGSIDE the existing ones - we
	// don't tear them down anymore. The previous "kill on
	// out-of-window seek" policy was correct for a single-client
	// orchestrator but broke as soon as two clients sat on different
	// positions of the same media (the second seek killed the
	// first's rafale). Concurrency is bounded by MaxRafalesPerHash;
	// past the cap the LRU rafale is evicted by evictLRUIfFullLocked
	// after the new spawn.
	//
	// When SingleVariant is ON, reachableSessionLocked additionally
	// filters by variantIdx so per-rung sessions stay isolated.
	// Claim the spawn under the SAME lock that decides nobody covers this
	// position. Without the claim, the only dedup was the re-check AFTER
	// process.Start (see spawnSession): every racer resolved the source, took a
	// video slot and exec'd a real ffmpeg, then all but one killed theirs
	// milliseconds later. On a 4K HEVC HDR ladder that is three concurrent
	// software decodes fighting over the box during the exact window the viewer
	// is waiting on segment 0 - prod, 2026-09-08: three `hls: ffmpeg start` at
	// 23:06:18.316/.317/.317 for one surviving rafale. The post-Start check
	// stays as the backstop for the cases this exact-window claim doesn't cover.
	o.mu.Lock()
	session, far, farPoint := o.reachableSessionLocked(hash, variantIdx, segmentIdx)
	var claim *pendingSpawn
	if session == nil {
		if winner := o.pendingSpawnCoversLocked(hash, variantIdx, segmentIdx); winner != nil {
			o.mu.Unlock()
			// A sibling is already starting the ffmpeg that will produce this
			// segment. Wait for ITS outcome rather than returning nil blind: a
			// spawn refused by the video limiter must still reach this caller as
			// ErrVideoCapacityExhausted, so the handler sheds with a 503 instead
			// of parking the viewer on a segment nobody is encoding. The wait is
			// bounded by spawnSession's own work (resolve + exec), which is the
			// work this goroutine would otherwise have duplicated.
			select {
			case <-winner.done:
				return winner.err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		// Coalesce rapid unserved seeks. `far == nil` here means no live rafale
		// even reaches this position - a genuine seek to fresh ground, which is
		// the storm case: on a heavy rung each such spawn is LRU-evicted by the
		// next seek before it writes its first segment. Defer it by
		// SeekCoalesceWindow and let a later seek for the same hash replace the
		// target; the last one standing spawns once, off the debounce timer. The
		// `far != nil` path (a nearby rafale to ride or overtake) is left
		// immediate - it is a continuation, not the storm, and must not stall.
		if o.opts.SeekCoalesceWindow > 0 && far == nil {
			o.coalesceSeekLocked(hash, fp, variantIdx, segmentIdx)
			o.mu.Unlock()
			// The caller blocks on cache.Wait as always; the deferred spawn
			// (or a later seek's) produces the segment. A seek the client has
			// already abandoned (the next seek aborted its fetch) simply never
			// gets waited on - no encoder was burned for it.
			return nil
		}
		claim = o.claimPendingSpawnLocked(hash, variantIdx, segmentIdx)
	}
	o.mu.Unlock()

	if session != nil {
		session.mu.Lock()
		inWindow := segmentIdx >= session.startSeg && segmentIdx <= session.endSeg
		nearAhead := segmentIdx > session.endSeg && segmentIdx <= session.endSeg+o.opts.LookAheadSegments
		session.lastAccess = time.Now()
		session.clientRead = true
		if segmentIdx > session.clientHighWater {
			session.clientHighWater = segmentIdx
		}
		if nearAhead {
			session.endSeg = segmentIdx + o.opts.LookAheadSegments
		}
		session.mu.Unlock()
		_ = inWindow // already encoded in the cover check; kept here for log clarity if added later
		return nil
	}

	// No rafale reaches this position - spawn a new one. If we cross
	// MaxRafalesPerHash, the post-spawn evictLRUIfFullLocked pass
	// retires the LRU.
	if far != nil {
		o.logger.Info("orchestrator: covering rafale too far behind, spawning at the requested segment",
			slog.String("hash", hash),
			slog.Int("variant", variantIdx),
			slog.Int("requested", segmentIdx),
			slog.Int("production_point", farPoint),
			slog.Int("reach", o.opts.SeekReachWindowSegs))
	} else {
		o.logger.Info("orchestrator: spawning additional rafale for unserved seek",
			slog.String("hash", hash),
			slog.Int("requested", segmentIdx))
	}
	err := o.spawnSession(ctx, fp, variantIdx, segmentIdx)
	if err != nil && far != nil && (errors.Is(err, ErrVideoCapacityExhausted) || errors.Is(err, ErrCapacityExceeded)) {
		// No room for a second encoder: ride the far one (slow, but not a 503).
		o.logger.Info("orchestrator: no capacity for a closer rafale, riding the far one",
			slog.String("hash", hash),
			slog.Int("requested", segmentIdx),
			slog.Int("production_point", farPoint))
		far.mu.Lock()
		far.lastAccess = time.Now()
		far.clientRead = true
		if segmentIdx > far.clientHighWater {
			far.clientHighWater = segmentIdx
		}
		if segmentIdx > far.endSeg {
			far.endSeg = segmentIdx + o.opts.LookAheadSegments
		}
		far.mu.Unlock()
		o.settlePendingSpawn(hash, claim, nil)
		return nil
	}
	// Publish the outcome to whoever waited on this claim, then drop it. Runs on
	// every path out of spawnSession - a stranded claim would wedge the hash for
	// the lifetime of the process.
	o.settlePendingSpawn(hash, claim, err)
	return err
}

// pendingSpawn records a spawn that has passed the covering-session check but
// whose ffmpeg is not registered in o.sessions yet. Its window is the one
// spawnSession will install, so a racer asking for a segment inside it waits on
// `done` and reads `err` instead of exec'ing a second encoder for the same
// bytes. err is written once, before done is closed, so reading it after the
// channel is a data race only in theory.
type pendingSpawn struct {
	variantIdx int
	startSeg   int
	endSeg     int
	done       chan struct{}
	err        error
}

// pendingSpawnCoversLocked returns the in-flight spawn that already promises
// segmentIdx, or nil. Deliberately the plain [startSeg, endSeg] window and NOT
// the nearAhead extension reachableSessionLocked grants: a pending spawn
// cannot widen its window on our behalf (it isn't registered yet), so claiming a
// segment past it would promise bytes nobody produces. Beyond the window we
// spawn a second rafale exactly as before. Caller holds o.mu.
func (o *SegmentOrchestrator) pendingSpawnCoversLocked(hash string, variantIdx, segmentIdx int) *pendingSpawn {
	for _, p := range o.pendingSpawns[hash] {
		if o.opts.SingleVariant && p.variantIdx != variantIdx {
			continue
		}
		// Same reach rule as a live rafale; nothing produced yet, point = startSeg-1.
		if segmentIdx >= p.startSeg && segmentIdx <= p.endSeg && segmentIdx-(p.startSeg-1) <= o.opts.SeekReachWindowSegs {
			return p
		}
	}
	return nil
}

// claimPendingSpawnLocked registers the window spawnSession is about to open and
// returns the claim to settle once it has. Caller holds o.mu.
func (o *SegmentOrchestrator) claimPendingSpawnLocked(hash string, variantIdx, startSeg int) *pendingSpawn {
	if o.pendingSpawns == nil {
		o.pendingSpawns = map[string][]*pendingSpawn{}
	}
	p := &pendingSpawn{
		variantIdx: variantIdx,
		startSeg:   startSeg,
		endSeg:     startSeg + o.opts.LookAheadSegments - 1,
		done:       make(chan struct{}),
	}
	o.pendingSpawns[hash] = append(o.pendingSpawns[hash], p)
	return p
}

// settlePendingSpawn records the spawn's outcome, wakes every racer parked on
// it and drops the claim. Nil claim = this call never took one (a live session
// covered the request), so there is nothing to settle.
func (o *SegmentOrchestrator) settlePendingSpawn(hash string, claim *pendingSpawn, err error) {
	if claim == nil {
		return
	}
	o.mu.Lock()
	list := o.pendingSpawns[hash]
	for i, p := range list {
		if p == claim {
			o.pendingSpawns[hash] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(o.pendingSpawns[hash]) == 0 {
		delete(o.pendingSpawns, hash)
	}
	// Written under mu and read only after done closes: the close is the
	// happens-before edge every waiter crosses.
	claim.err = err
	o.mu.Unlock()
	close(claim.done)
}

// pendingSeek is the debounced spawn of a burst of unserved seeks for one
// fingerprint. Only the LAST target survives (rapid seeks overwrite it); the
// single timer fires spawnSession once the window has been quiet for
// SeekCoalesceWindow. Guarded by o.mu.
type pendingSeek struct {
	fp         DecisionFingerprint
	variantIdx int
	segmentIdx int
	// deadline is pushed out on every new seek. The timer re-arms itself for the
	// remaining time when it fires early (a seek landed after it was scheduled),
	// which is what makes "last seek wins" hold without racing timer.Reset from
	// two goroutines: Reset is only ever called from fireCoalescedSeek, under
	// o.mu, on an already-expired timer.
	deadline time.Time
	timer    *time.Timer
}

// coalesceSeekLocked records segmentIdx as the latest unserved-seek target for
// hash and (re)arms the debounce. Last write wins. Caller holds o.mu.
func (o *SegmentOrchestrator) coalesceSeekLocked(hash string, fp DecisionFingerprint, variantIdx, segmentIdx int) {
	if o.pendingSeeks == nil {
		o.pendingSeeks = map[string]*pendingSeek{}
	}
	ps := o.pendingSeeks[hash]
	if ps == nil {
		ps = &pendingSeek{}
		o.pendingSeeks[hash] = ps
	}
	ps.fp = fp
	ps.variantIdx = variantIdx
	ps.segmentIdx = segmentIdx
	ps.deadline = time.Now().Add(o.opts.SeekCoalesceWindow)
	if ps.timer == nil {
		ps.timer = time.AfterFunc(o.opts.SeekCoalesceWindow, func() { o.fireCoalescedSeek(hash) })
	}
	// No Reset here: a newer seek only moves the deadline, and fireCoalescedSeek
	// re-arms the timer for the remaining slice when it wakes too early.
	o.logger.Debug("orchestrator: coalescing unserved seek",
		slog.String("hash", hash),
		slog.Int("variant", variantIdx),
		slog.Int("target", segmentIdx))
}

// fireCoalescedSeek is the debounce timer callback: it spawns ONE rafale at the
// last recorded target once the window has been quiet, or re-arms if a newer
// seek pushed the deadline out. Runs detached (off the timer goroutine).
func (o *SegmentOrchestrator) fireCoalescedSeek(hash string) {
	o.mu.Lock()
	ps := o.pendingSeeks[hash]
	if ps == nil {
		o.mu.Unlock()
		return
	}
	if remaining := time.Until(ps.deadline); remaining > 0 {
		// A seek landed after this timer was scheduled: wait out the new window.
		ps.timer.Reset(remaining)
		o.mu.Unlock()
		return
	}
	fp, variantIdx, segmentIdx := ps.fp, ps.variantIdx, ps.segmentIdx
	delete(o.pendingSeeks, hash)
	// Bail if the orchestrator is shutting down - shutdownCtx is already
	// cancelled, so the spawn would only resolve-then-fail. Close stops timers
	// too, but a fire racing shutdownCancel lands here.
	if o.shutdownCtx.Err() != nil {
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()

	// Spawn detached: the request that armed this is likely long gone, so use
	// the orchestrator's long-lived context (as PreWarm does). ResolveSource
	// only needs the media id (already in fp), not the request's auth context.
	ctx, cancel := context.WithTimeout(o.shutdownCtx, 30*time.Second)
	defer cancel()

	// Re-check under the lock: a rafale may have appeared (a `far`-path spawn, a
	// concurrent hash) or the segment may have landed while we debounced.
	if o.cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: variantIdx, SegmentIdx: segmentIdx}, fp.UseFMP4) {
		return
	}
	o.mu.Lock()
	if s, _, _ := o.reachableSessionLocked(hash, variantIdx, segmentIdx); s != nil {
		o.mu.Unlock()
		return
	}
	if o.pendingSpawnCoversLocked(hash, variantIdx, segmentIdx) != nil {
		o.mu.Unlock()
		return
	}
	claim := o.claimPendingSpawnLocked(hash, variantIdx, segmentIdx)
	o.mu.Unlock()

	o.logger.Info("orchestrator: spawning coalesced rafale for unserved seek",
		slog.String("hash", hash),
		slog.Int("variant", variantIdx),
		slog.Int("requested", segmentIdx))
	err := o.spawnSession(ctx, fp, variantIdx, segmentIdx)
	o.settlePendingSpawn(hash, claim, err)
	if err != nil {
		o.logger.Info("orchestrator: coalesced seek spawn failed (client retry re-drives it)",
			slog.String("hash", hash),
			slog.Int("requested", segmentIdx),
			slog.Any("err", err))
	}
}

// acquireSessionSlot reserves one of the MaxActiveSessions global rafale slots
// without blocking. Returns true when a slot was taken (or the orchestrator is
// uncapped). A false return means the host is at capacity and the caller must
// refuse the spawn with ErrCapacityExceeded.
func (o *SegmentOrchestrator) acquireSessionSlot() bool {
	if o.sessionSem == nil {
		return true
	}
	select {
	case o.sessionSem <- struct{}{}:
		return true
	default:
		return false
	}
}

// releaseSessionSlot frees a global rafale slot taken by acquireSessionSlot.
// No-op when uncapped. Must be called at most once per successful acquire; the
// non-blocking drain is defensive so a double-release can never deadlock.
func (o *SegmentOrchestrator) releaseSessionSlot() {
	if o.sessionSem == nil {
		return
	}
	select {
	case <-o.sessionSem:
	default:
	}
}

// spawnSession launches an ffmpeg rafale for the given fingerprint at
// segmentIdx, producing up to startSeg + LookAheadSegments segments. Called
// under no lock; takes o.mu briefly to register the session.
//
// variantIdx identifies which ABR rung to encode. When SingleVariant is OFF,
// it is only stored on the session for the spawn-race check and the ffmpeg
// invocation encodes ALL profiles (multi-variant). When SingleVariant is ON,
// only fp.Profiles[variantIdx] is passed to the ffmpeg builder so a single
// encode is produced for that rung.
func (o *SegmentOrchestrator) spawnSession(
	ctx context.Context,
	fp DecisionFingerprint,
	variantIdx int,
	startSegIdx int,
) error {
	hash := fp.Hash()
	endSegIdx := startSegIdx + o.opts.LookAheadSegments - 1

	// Global concurrency backstop: refuse a fresh rafale when the host is already
	// running MaxActiveSessions of them. The per-hash cap (MaxRafalesPerHash)
	// bounds rafales for ONE media but not the TOTAL, so without this a client can
	// melt a weak CPU by fanning out across fingerprints. The reap goroutine frees
	// this slot when the session exits; every early-return below frees it via
	// releaseSlot() since ownership only transfers to the reap goroutine once it
	// starts.
	if !o.acquireSessionSlot() {
		metrics.HLSVODSessionsRejected.Inc()
		o.logger.Warn("orchestrator: max active sessions reached, refusing spawn",
			slog.String("hash", hash),
			slog.Int("cap", o.opts.MaxActiveSessions))
		return ErrCapacityExceeded
	}
	slotReleased := false
	releaseSlot := func() {
		if !slotReleased {
			o.releaseSessionSlot()
			slotReleased = true
		}
	}

	// Resolve source through the request context (auth + db calls live there)
	// but spawn ffmpeg under the orchestrator's long-lived context so it
	// survives past the original request returning to the client.
	srcPath, _, err := o.source.ResolveSource(ctx, fp)
	if err != nil {
		releaseSlot()
		return fmt.Errorf("spawnSession: resolve source: %w", err)
	}
	// Phase 3 will wire real streams (burn-in / HDR tonemap); for Phase 1 the
	// ffmpeg adapter's defaults are sufficient for video + audio.

	outDir, err := o.cache.DirFor(hash)
	if err != nil {
		releaseSlot()
		return fmt.Errorf("spawnSession: cache dir: %w", err)
	}

	teardown := o.cache.RegisterSession(hash)

	// Resolve the profile list for this session.
	// Single-variant ON: encode only the one rung the client asked for
	// (1 decode + 1 encode, saves the unused rungs).
	// Single-variant OFF: encode the full ABR ladder in one multi-output
	// ffmpeg - byte-identical to the pre-flag behaviour.
	var sessionProfiles []transcoding.HLSProfile
	var sessionQuality []transcoding.QualityTarget
	var sessionVariantIdx int
	if o.opts.SingleVariant {
		if variantIdx < 0 || variantIdx >= len(fp.Profiles) {
			teardown()
			releaseSlot()
			return fmt.Errorf("spawnSession: variantIdx %d out of range (profiles=%d)", variantIdx, len(fp.Profiles))
		}
		sessionProfiles = profilesToDomain(fp.Profiles[variantIdx : variantIdx+1])
		sessionQuality = qualityTargetsSlice(fp.QualityTargets, variantIdx, variantIdx+1)
		sessionVariantIdx = variantIdx
	} else {
		sessionProfiles = profilesToDomain(fp.Profiles)
		sessionQuality = qualityTargetsSlice(fp.QualityTargets, 0, len(fp.Profiles))
		sessionVariantIdx = 0 // multi-variant: VariantIndex unused by the multi-variant builder
	}

	progressLogDir := ""
	if o.opts.FFmpegProgressLogDir != nil {
		progressLogDir = o.opts.FFmpegProgressLogDir(ctx)
	}
	procOpts := transcoding.HLSProcessOpts{
		OutputDir:          outDir,
		FilePath:           srcPath,
		ProgressLogPath:    ffmpegProgressLogPath(progressLogDir, hash, sessionVariantIdx, startSegIdx),
		Profiles:           sessionProfiles,
		QualityTargets:     sessionQuality,
		VariantIndex:       sessionVariantIdx,
		VideoDecision:      "transcode",
		AudioDecision:      fp.AudioDecision(),
		AudioEncoder:       fp.AudioEncoder,
		AudioTrackIndex:    fp.AudioTrack,
		AudioUserOffsetSec: float64(fp.AudioOffsetMs) / 1000.0,
		SubtitleTrackIndex: fp.SubtitleBurnInTrack,
		SubtitleCodec:      fp.SubtitleBurnInCodec,
		HDRSupported:       false,         // tonemap is part of decision; HDRSupported=false on the client forces tonemap
		ToneMap:            fp.HDRTonemap, // HDR->SDR tonemap decision, carried as a bool (Streams not threaded here)
		HDRPassthrough:     fp.HDRPassthrough,
		HDRTransfer:        fp.HDRTransfer,
		AudioNormalize:     fp.AudioNormalize,
		StartSec:           float64(startSegIdx) * o.opts.SegmentSeconds,
		// StartNumber is what tells the transcoder this rafale is a SEEK whose
		// fMP4 fragments come out 0-based and must be rebased to the absolute
		// timeline before they reach the shared cache. It was never set here:
		// every seek or resume into a transcoded title published fragments
		// stamped from 0 under absolute segment names, hls.js placed them at
		// the wrong position, and the player stayed in buffering (prod,
		// 2026-09-02/03 - the segment cache then carried them to every viewer).
		StartNumber:         startSegIdx,
		Streams:             nil, // HDR tonemap is wired via ToneMap; bitmap burn-in via SubtitleCodec (reads 0:s:N from the source input directly, no Streams needed)
		VideoCodec:          fp.VideoCodec,
		AudioCodec:          fp.AudioCodec,
		MaxAudioChannels:    fp.MaxAudioChannels,
		SourceAudioChannels: fp.SourceAudioChannels,
		ForceSidecarSubs:    fp.ForceSidecarSubs,
		// The idet content verdict, for a source whose container FLAG lies about
		// being progressive. It only ever reaches a software deinterlacer
		// (deinterlaceFilterFor), which is why this path could omit it while
		// every HW session deinterlaced on the GPU off the decoder's flag.
		// The decoupled route brought bwdif back onto this path, and a lying
		// flag is a legacy-rip trait - exactly that route's population.
		DeinterlaceAll:  fp.DeinterlaceAll,
		InverseTelecine: fp.Telecine,
		MaxH264Level:    fp.MaxH264Level,
		// fp.UseFMP4 is the container choice baked into the manifest the
		// player will receive (.m4s vs .ts). Without forwarding it, ffmpeg
		// always wrote .ts via the multi-variant builder while the manifest
		// pointed at .m4s - segment fetches missed the cache forever.
		UseFMP4: fp.UseFMP4,
		// Hardware encode backend resolved by the handler (empty = software).
		// The ffmpeg builder switches to the zero-copy GPU pipeline only when
		// HWAccel is a valid non-None backend AND the content is simple.
		HWAccel:  transcoding.HWAccelType(fp.HWAccel),
		HWDevice: fp.HWDevice,
		// Decoupled route: this source's codec is one the GPU cannot decode, so
		// ffmpeg decodes it in software and uploads the frames for the GPU to
		// scale and encode. Everything downstream of the upload is unchanged.
		HWSoftwareDecode: fp.HWSoftwareDecode,
		HWFeatures:       fp.HWFeatures,
		HWTonemap:        fp.HWTonemap,     // GPU HDR->SDR fold (resolved by the handler; false -> software tonemap)
		HWOverlay:        fp.HWOverlay,     // GPU bitmap-subtitle blend (false -> the burn-in keeps the software path)
		HWDeinterlace:    fp.HWDeinterlace, // QSV session-level deinterlacer, only on a known-interlaced source
		PerVariantHW:     o.opts.HWPerVariant,
	}

	// Video-encode capacity gate: every session this orchestrator spawns
	// encodes video (there is no copy/remux mode on the HW path), so it is
	// unconditionally gated when a limiter is wired. Acquired BEFORE ffmpeg
	// spawns (the actual CPU/GPU-consuming action) and released on every exit
	// path below exactly once (see videoSlotAcquired / releaseVideoSlot).
	videoSlotAcquired := false
	if o.videoLimiter != nil {
		if !o.videoLimiter.TryAcquireRoute(hash, LadderClassOf(fp), topProfileHeight(sessionProfiles), SourceClassOf(fp), RouteCostKeyOf(fp), fp.SourceFPS) {
			teardown()
			releaseSlot()
			metrics.HLSVODSessionsRejected.Inc()
			o.logger.Warn("orchestrator: max video encode sessions reached, refusing spawn",
				slog.String("hash", hash))
			return ErrVideoCapacityExhausted
		}
		videoSlotAcquired = true
	}

	sessionCtx, cancel := context.WithCancel(o.shutdownCtx)
	handle, err := o.process.Start(sessionCtx, procOpts)
	if err != nil {
		cancel()
		teardown()
		releaseSlot()
		if videoSlotAcquired {
			o.videoLimiter.Release(hash)
		}
		return fmt.Errorf("spawnSession: ffmpeg start: %w", err)
	}

	session := &orchestratorSession{
		hash:              hash,
		mediaID:           fp.MediaID,
		handle:            handle,
		startSeg:          startSegIdx,
		endSeg:            endSegIdx,
		lastAccess:        time.Now(),
		teardown:          teardown,
		cancelCtx:         cancel,
		done:              make(chan struct{}),
		clientHighWater:   startSegIdx,
		producedHighWater: startSegIdx - 1,
		useFMP4:           fp.UseFMP4,
		variantIdx:        variantIdx,
		videoSlotAcquired: videoSlotAcquired,
	}

	o.mu.Lock()
	// Multi-rafale spawn race: only veto the new spawn if another
	// goroutine has ALREADY spawned a rafale that covers startSegIdx
	// (window or nearAhead). Multiple rafales on disjoint ranges of
	// the same hash are the whole point of this refactor - letting
	// them coexist is what fixes the multi-client / multi-tab bug.
	// In single-variant mode the race check also filters by variantIdx
	// so two goroutines requesting the same rung at the same position
	// are correctly deduplicated.
	// Reach applies here too: a far-behind sibling must not veto the spawn made to escape it.
	if existing, _, _ := o.reachableSessionLocked(hash, variantIdx, startSegIdx); existing != nil {
		o.mu.Unlock()
		o.logger.Info("orchestrator: spawn race lost (covered by sibling rafale), discarding new session",
			slog.String("hash", hash),
			slog.Int("start_seg", startSegIdx))
		cancel()
		if err := handle.Stop(); err != nil {
			o.logger.Debug("orchestrator: stop on spawn race lost (ignored)", slog.Any("err", err))
		}
		teardown()
		releaseSlot()
		// This session never reaches the reap goroutine or killSession (it was
		// discarded before either could take ownership), so release its video
		// slot here - the only place that will ever do so for it.
		o.releaseVideoSlot(session)
		return nil
	}
	o.sessions[hash] = append(o.sessions[hash], session)
	o.spawnedHashes[hash] = struct{}{}
	// Count the session live BEFORE anything can reap it. The reap goroutine
	// below fires metricDecOnce as soon as ffmpeg exits, and an ffmpeg that dies
	// instantly (codec refused, source gone, GPU dead) would consume the Once
	// before this Inc ran - stranding the gauge one above zero forever and
	// silencing the very leak detector metricDecOnce exists to feed.
	metrics.HLSVODSessions.Inc()
	evicted := o.evictLRUIfFullLocked(hash)
	o.mu.Unlock()

	// LRU-evict the oldest rafale outside the lock - killSession
	// reacquires o.mu so we can't hold it here.
	if evicted != nil {
		o.logger.Info("orchestrator: LRU-evicting rafale (cap reached)",
			slog.String("hash", hash),
			slog.Int("evicted_start_seg", evicted.startSeg))
		o.killSession(evicted, "lru_cap")
	}

	// Reap when ffmpeg exits so the session entry doesn't linger
	// forever. Releases its cache-pending refcount via teardown too -
	// natural EOF skips killSession's teardown path so without this,
	// future Wait() callers for the hash would block until the cache
	// pending channel was eventually closed by another route.
	// Refcount is idempotent (the release func no-ops on second call)
	// so a path that goes through both reap AND killSession remains
	// safe.
	//
	// Ownership of the global concurrency slot transfers to the reap goroutine
	// here: it runs exactly once per spawned session and frees the slot when the
	// session has fully exited. Mark it released so the function-scoped
	// releaseSlot() can't double-free if more early returns are added later.
	slotReleased = true
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(sessionCtx, "segment_orchestrator: detached reap goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
			}
		}()
		// Distinguish a natural ffmpeg exit (inspect why) from our own teardown
		// (seek-cancel / GC / shutdown via sessionCtx) - only the former can be
		// a hardware-capability failure worth demoting on.
		naturalExit := false
		select {
		case <-handle.Done():
			naturalExit = true
		case <-sessionCtx.Done():
		}
		session.mu.Lock()
		if session.killed {
			naturalExit = false
		}
		session.mu.Unlock()
		close(session.done)
		session.mu.Lock()
		teardown := session.teardown
		session.teardown = nil
		session.mu.Unlock()
		if teardown != nil {
			teardown()
		}
		o.removeSessionFromList(hash, session)
		// Natural exit: decrement the live-sessions gauge (exactly once - see
		// killSession). The reap is the dominant exit path (ffmpeg hits EOF).
		// Count natural completions so the gauge can be reconciled against
		// spawns − (completed + killed): a divergence flags a teardown leak like
		// the one this Once was added to fix. naturalExit + killSession's
		// HLSVODSessionsKilled are mutually exclusive (a forced kill wakes the
		// reap via sessionCtx, leaving naturalExit false).
		session.metricDecOnce.Do(func() {
			metrics.HLSVODSessions.Dec()
			if naturalExit {
				metrics.HLSVODSessionsCompleted.Inc()
			}
		})

		// The fallback ladder of a failed GPU session, in order:
		//   1. the SESSION is gone (ffmpeg exited);
		//   2. the TITLE is sent to the CPU route (blockTitleRoute) for every
		//      deterministic class - a media/route incompatibility, a refused
		//      option, a missing encoder, a dead device all replay identically
		//      on the same title, and the client's retry must play;
		//   3. the HOST is only demoted by the calibration service, once a
		//      media-independent probe confirms the failure is the GPU's
		//      (hwDemote -> DemoteCapability). A route incompatibility never
		//      touches the host: it is recorded there and nothing else.
		// Cancellation / transient / unknown are ignored. A session that
		// exited on its own without writing a single segment is a silent HW
		// death the stderr classifier cannot see: a driver crash (SIGSEGV
		// inside the GPU .so / a TDR GPU reset on WSL d3d12) or an exit-0
		// "Nothing was written" - the title falls back, and the host is asked
		// to prove itself as for an init failure.
		// The title block is set HERE, synchronously, so the client's retry -
		// which may already be in flight - resolves to the CPU route at once.
		// The host proof (hwDemote -> DemoteCapability's synthetic encodes, up
		// to several seconds) runs on its own goroutine, after the slots below
		// are freed: revalidating the host must never hold a session slot or
		// delay the session fallback.
		var hostVerdict func()
		if naturalExit && procOpts.HWAccel != "" {
			class := handle.Failure()
			switch {
			case class.BlocksTitleRoute():
				o.blockTitleRoute(fp.MediaID, hash, procOpts.HWAccel, string(class))
				if o.hwDemote != nil {
					hostVerdict = func() { o.hwDemote(o.shutdownCtx, hwFailure(class, fp)) }
				}
			case class.SilentDeathCandidate() && !o.hashProducedSegment(hash, variantIdx, fp.UseFMP4):
				o.logger.Warn("orchestrator: HW session produced no segments",
					slog.String("hash", hash),
					slog.String("class", string(class)),
					slog.String("hw_backend", string(procOpts.HWAccel)),
					slog.String("codec", fp.VideoCodec))
				o.blockTitleRoute(fp.MediaID, hash, procOpts.HWAccel, "no_segment_produced/"+string(class))
				if o.hwDemote != nil {
					hostVerdict = func() { o.hwDemote(o.shutdownCtx, hwFailure(transcoding.FailureHWInit, fp)) }
				}
			}
		}

		// Free the global concurrency slot: this session has fully exited and is
		// off the live list, so a queued spawn may now proceed.
		o.releaseSessionSlot()
		// Natural exit: killSession never ran for this session, so free its video
		// slot here. Once-guarded (see releaseVideoSlot) so a concurrent kill that
		// raced this reap can't double-release.
		o.releaseVideoSlot(session)
		if hostVerdict != nil {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.ErrorContext(o.shutdownCtx, "segment_orchestrator: host verdict goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
					}
				}()
				hostVerdict()
			}()
		}
	}()

	// HW fast-fail watchdog: a hardware rafale that hasn't written its init
	// segment within HWFirstSegmentTimeout is a stuck/dead GPU pipeline (a TDR
	// reset, a hung encoder). Don't make the client wait out the full ~25 s
	// segment timeout - kill the session so the retry re-resolves (the GPU
	// again: one missed init is transient, not a title verdict). The init is the
	// signal, not a full segment: a working-but-slow GPU writes it within
	// ~1-3 s while a dead one never does, so the tight window doesn't false-
	// fail a slow weak-NAS encoder. Only HW sessions, only when a demote sink
	// is wired (otherwise a kill would just re-spawn the same broken HW).
	if procOpts.HWAccel != "" && o.opts.HWFirstSegmentTimeout > 0 && o.hwDemote != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(sessionCtx, "segment_orchestrator: detached HW watchdog goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
				}
			}()
			t := time.NewTimer(o.opts.HWFirstSegmentTimeout)
			defer t.Stop()
			select {
			case <-session.done: // already exited - the reap goroutine handles it
				return
			case <-sessionCtx.Done(): // cancelled (seek / GC / shutdown)
				return
			case <-t.C:
			}
			if o.hwSessionAlive(hash, variantIdx, fp.UseFMP4) {
				return // the encoder is producing (maybe slowly) - let it run
			}
			o.logger.Warn("orchestrator: HW session wrote no init within watchdog window, title falls back to the CPU route",
				slog.String("hash", hash),
				slog.String("hw_backend", string(procOpts.HWAccel)),
				slog.String("codec", fp.VideoCodec),
				slog.Duration("window", o.opts.HWFirstSegmentTimeout))
			// A missed init on its own is TRANSIENT (a slow start under boot load
			// as often as a dead GPU): the title is not blocked, the retry is free
			// to take the GPU again. The host proves itself off this goroutine's
			// critical path, after the kill, and only its own probe can demote it.
			o.killSession(session, "hw watchdog: no init segment")
			o.hwDemote(o.shutdownCtx, hwFailure(transcoding.FailureHWInit, fp))
		}()
	}

	o.logger.Info("orchestrator: rafale spawned",
		slog.String("hash", hash),
		slog.Int("start_seg", startSegIdx),
		slog.Int("end_seg", endSegIdx),
		slog.String("source", srcPath))

	metrics.HLSVODRafaleSpawned.WithLabelValues("miss").Inc()

	return nil
}

// hashProducedSegment reports whether any media segment for the given variantIdx
// of hash has landed on disk. Used by the reap goroutine to tell a working HW
// session apart from a silent HW death (a process that exits without writing a
// single segment). In multi-variant mode variantIdx=0 (the base rung) is always
// produced first; in single-variant mode the rung index is the session's own
// variantIdx.
func (o *SegmentOrchestrator) hashProducedSegment(hash string, variantIdx int, useFMP4 bool) bool {
	idxs, err := o.cache.ListReady(hash, variantIdx, useFMP4)
	return err == nil && len(idxs) > 0
}

// hwSessionAlive reports whether a session has shown any sign the encoder is
// producing output: a non-empty fMP4 init segment (written within ~1-3 s by a
// working pipeline, never by a stuck/dead GPU) or at least one media segment.
// Used by the HW fast-fail watchdog to tell a slow-but-working GPU from a dead
// one without waiting for a full first segment.
// variantIdx selects which rung's init/segment to check - in multi-variant mode
// this is always 0 (base rung); in single-variant mode it is the session's own rung.
func (o *SegmentOrchestrator) hwSessionAlive(hash string, variantIdx int, useFMP4 bool) bool {
	if o.cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: variantIdx, IsInit: true}, useFMP4) {
		return true
	}
	return o.hashProducedSegment(hash, variantIdx, useFMP4)
}

// closestCoveringSessionLocked: the covering rafale started closest below
// segmentIdx. Rafales of a hash share one cache dir, so a segment on disk was
// written by that one, not by the first cover listed. Caller holds o.mu.
func (o *SegmentOrchestrator) closestCoveringSessionLocked(hash string, variantIdx, segmentIdx int) *orchestratorSession {
	var best *orchestratorSession
	bestStart := -1
	for _, s := range o.sessions[hash] {
		if o.opts.SingleVariant && s.variantIdx != variantIdx {
			continue
		}
		s.mu.Lock()
		dead := s.cancelCtx == nil
		inWindow := segmentIdx >= s.startSeg && segmentIdx <= s.endSeg
		start := s.startSeg
		s.mu.Unlock()
		if dead || !inWindow {
			continue
		}
		if start > bestStart {
			best, bestStart = s, start
		}
	}
	return best
}

// touchSession bumps lastAccess + client high-water on the rafale that
// covers segmentIdx, falling back to the most-recently-accessed
// rafale of the hash when no exact cover exists (cache-hit case: the
// client is fetching from disk, but we still want to keep at least
// one rafale warm for the upcoming segments). Used on cache hits so
// the GC doesn't reap a session whose segments are still being
// consumed from the cache, and so the throttle loop sees client
// progress and resumes a paused ffmpeg in time.
//
// variantIdx is forwarded to closestCoveringSessionLocked so that in
// single-variant mode the right per-rung session is touched.
func (o *SegmentOrchestrator) touchSession(hash string, variantIdx, segmentIdx int) {
	o.mu.Lock()
	target := o.closestCoveringSessionLocked(hash, variantIdx, segmentIdx)
	covered := target != nil
	if target == nil {
		// No exact cover. Bump the most-recently-touched rafale so the
		// GC keeps at least one session warm for the hash.
		var newest time.Time
		for _, s := range o.sessions[hash] {
			s.mu.Lock()
			la := s.lastAccess
			s.mu.Unlock()
			if target == nil || la.After(newest) {
				target = s
				newest = la
			}
		}
	}
	o.mu.Unlock()
	if target == nil {
		return
	}
	target.mu.Lock()
	target.lastAccess = time.Now()
	target.clientRead = true
	if segmentIdx > target.clientHighWater {
		target.clientHighWater = segmentIdx
	}
	// A cache hit proves production through segmentIdx (the throttle only samples once a second).
	if covered && segmentIdx > target.producedHighWater {
		target.producedHighWater = segmentIdx
	}
	target.mu.Unlock()
}

// ActiveCount returns how many ffmpeg sessions are currently live across
// every fingerprint. Test + observability accessor; cheap to call.
// Counts all rafales in the slice, not just distinct hashes - a film
// with two concurrent clients reports 2, not 1.
func (o *SegmentOrchestrator) ActiveCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, list := range o.sessions {
		n += len(list)
	}
	return n
}

// ActiveSessionCount is the alias the library trickplay scheduler and the
// admin/system metrics gauge consume - same shape as the legacy HLSManager's
// counter, so the trickplay ActiveStreamChecker contract drops in without
// changing callers.
func (o *SegmentOrchestrator) ActiveSessionCount() int { return o.ActiveCount() }

// RemoveSession kills every rafale currently producing for the given media
// ID. A single media can have several live sessions when different clients
// negotiate different audio tracks / sub burn-ins / codecs (one DecisionHash
// each), so the legacy contract - keyed by mediaID - fans out here. Safe to
// call for a media with no live session (no-op). Implements the
// HLSSessionRemover interface the admin/kill-session and playback-stop paths
// rely on.
func (o *SegmentOrchestrator) RemoveSession(mediaItemID string) {
	o.mu.Lock()
	var toKill []*orchestratorSession
	for _, list := range o.sessions {
		for _, s := range list {
			s.mu.Lock()
			match := s.mediaID == mediaItemID
			s.mu.Unlock()
			if match {
				toKill = append(toKill, s)
			}
		}
	}
	o.mu.Unlock()
	for _, s := range toKill {
		o.killSession(s, "remove_session")
	}
}

// RemoveHash kills every rafale producing for ONE decision hash and reports
// how many it killed - the surgical counterpart of RemoveSession above, with
// the same contract as SoftwareProducer.RemoveHash: a viewer who changed a
// decision input (audio track, sub burn-in, offset) left this hash behind,
// while the media's OTHER hashes may still have a live viewer that a
// media-wide kill would take down with it. No-op on an empty or unknown hash.
//
// Sessions fetched within reclaimMinIdle are spared - see that constant.
func (o *SegmentOrchestrator) RemoveHash(hash, reason string) int {
	if hash == "" {
		return 0
	}
	cutoff := time.Now().Add(-reclaimMinIdle)
	o.mu.Lock()
	var toKill []*orchestratorSession
	for _, s := range o.sessions[hash] {
		s.mu.Lock()
		idle := s.lastAccess.Before(cutoff)
		s.mu.Unlock()
		if idle {
			toKill = append(toKill, s)
		}
	}
	o.mu.Unlock()
	for _, s := range toKill {
		o.killSession(s, reason)
	}
	return len(toKill)
}

// reclaimMinIdle is how long a session must have gone unfetched before a
// superseded-hash reclaim (RemoveHash, both producers) may kill it. The caller
// reads "nobody is on this hash" from the StreamHashRegistry, which only sees
// token-carrying streams: a JWT-authenticated client holds a real encoder while
// being invisible to it. Short on purpose - stretching it brings back the
// 2026-07-25 symptom, a track switch refused by the viewer's own dead encoder.
const reclaimMinIdle = 2 * time.Second

// Compile-time assertion: the orchestrator IS the hardware StreamProducer.
// It already carried EnsureSegmentRange / PreWarm / NotifyClientHint /
// ActiveSessionCount / RemoveSession / Close; Profiles + WaitBudget below
// complete the interface. The behaviour of every pre-existing method is
// unchanged - this is the HW impl behind the seam.
var _ StreamProducer = (*SegmentOrchestrator)(nil)

// WaitBudget is how long the segment handler should block on cache.Wait for an
// orchestrated (hardware) segment. The HW path bursts ahead and lands segment
// 0 within a few seconds on a working GPU, so 25 s is a generous worst-case
// cold-start window (it matches the handler's historical segmentWaitTimeout).
func (o *SegmentOrchestrator) WaitBudget() time.Duration { return o.opts.WaitBudgetDur }

// Profiles shapes the ABR ladder for the HARDWARE path - the domain's
// ShapeHardwareLadder (the ladder engine reaches it directly; this method
// serves the admission spill and the hand-built handlers). decision is unused
// here (the HW path keeps every rung); it is part of the interface so the
// software producer can branch on HDR.
func (o *SegmentOrchestrator) Profiles(base []transcoding.HLSProfile, src SourceDims, _ transcoding.PlaybackDecision, capacity transcoding.EncodeCapability) []transcoding.HLSProfile {
	return transcoding.ShapeHardwareLadder(base, src.facts(), capacity)
}

// qualityTargetsSlice is targets[lo:hi] when the policy answered for every
// rung, nil otherwise (the builders then apply the static policy).
func qualityTargetsSlice(targets []transcoding.QualityTarget, lo, hi int) []transcoding.QualityTarget {
	if hi > len(targets) || lo < 0 || lo > hi {
		return nil
	}
	return targets[lo:hi]
}

// profilesToDomain converts the cache fingerprint profiles into the domain
// type expected by the ffmpeg adapter. Both shapes carry essentially the same
// data (the domain type has extra fields we don't need in the fingerprint).
func profilesToDomain(ps []ProfileFingerprint) []transcoding.HLSProfile {
	out := make([]transcoding.HLSProfile, len(ps))
	for i, p := range ps {
		out[i] = transcoding.HLSProfile{
			Name:         p.Name,
			Width:        p.Width,
			Height:       p.Height,
			VideoBitrate: p.VideoBitrate,
			AudioBitrate: p.AudioBitrate,
		}
	}
	return out
}
