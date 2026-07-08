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

// OrchestratorOptions configures the orchestrator's lifecycle policy.
type OrchestratorOptions struct {
	// LookAheadSegments is the size of each ffmpeg rafale: when a request arrives
	// for segment X and no live session covers it, a fresh ffmpeg is spawned at
	// X and runs until it has encoded X + LookAheadSegments. Larger = fewer
	// ffmpeg restarts but more wasted work on seeks; default 30 (90 s at 3 s
	// segments) which empirically covers small rewinds + tight ABR switches
	// without paying for segments the user won't watch.
	LookAheadSegments int

	// InactivityTimeout is how long an idle session lingers before the GC kills
	// it. "Idle" = no segment request since LastAccess. Default 60 s, matching
	// hls.js' default fragment retry window.
	InactivityTimeout time.Duration

	// GCInterval is the period of the inactivity sweeper. Default 15 s.
	GCInterval time.Duration

	// SegmentSeconds — keep in sync with ManifestSegmentSeconds and the ffmpeg
	// force_key_frames cadence. Exposed for tests.
	SegmentSeconds float64

	// ThrottleEnabled toggles the pause-when-ahead policy (Plex pattern).
	// When ffmpeg has produced ThrottlePauseAhead segments more than the
	// client's high-water request, SIGSTOP the process; SIGCONT when the
	// client catches back up to within ThrottleResumeBelow segments.
	// Saves CPU + thermals + disk write churn on calm linear playback
	// without affecting seek latency (cancel-on-seek tears the session
	// down before throttle gets involved).
	ThrottleEnabled bool

	// ThrottlePauseAhead is the buffer (in segments) above which ffmpeg
	// gets paused. Default 10 = 30s buffer at 3s segments. Plex docs cite
	// 30-45s as the empirical sweet spot.
	ThrottlePauseAhead int

	// ThrottleResumeBelow is the buffer (in segments) below which a paused
	// session resumes. Hysteresis vs ThrottlePauseAhead avoids flapping.
	// Default 5 = 15s.
	ThrottleResumeBelow int

	// ThrottleTickInterval is how often the throttle policy is evaluated.
	// Default 1s — coarse enough to be free, tight enough that a seek-out
	// that lands on a paused session resumes within one tick.
	ThrottleTickInterval time.Duration

	// MaxRafalesPerHash bounds the number of concurrent ffmpeg rafales
	// that can be active for a single fingerprint. Each rafale serves a
	// disjoint span (one client per rafale, in practice). Default 3
	// is enough for "two browser tabs + one resume" type traffic on a
	// single film without melting the CPU on a server hosting 10
	// different films. Set to 1 to recover the legacy single-rafale
	// behaviour (kill-on-conflict).
	MaxRafalesPerHash int

	// MaxActiveSessions caps the TOTAL number of concurrent ffmpeg rafales the
	// orchestrator runs across ALL fingerprints, independent of
	// MaxRafalesPerHash (which only bounds rafales for a single media). It is the
	// global CPU backstop: without it an authenticated client can saturate a weak
	// host by requesting many distinct media — or diversifying one media's
	// decision hash (audio/sub/hdr query params) — to spawn N software x264
	// sessions. At the cap, spawnSession returns ErrCapacityExceeded (HTTP 503)
	// instead of starting ffmpeg. Zero = unlimited (no cap), preserving legacy
	// behaviour for tests and callers that don't set it.
	MaxActiveSessions int

	// HWFirstSegmentTimeout is the fast-fail watchdog for hardware sessions: if
	// a HW-encode rafale hasn't written its fMP4 init segment (the first sign the
	// encoder is actually producing output) within this window, it's treated as
	// a stuck/dead GPU pipeline — demoted to software and killed — instead of
	// letting the client wait out the full ~25 s segment timeout. The init
	// (vs a full segment) is the signal because a working-but-slow HW pipeline
	// writes it within ~1-3 s while a TDR-dead one never does, so a tight 5 s
	// default won't false-fail a slow weak-NAS GPU. Software sessions are
	// unaffected; zero disables the watchdog.
	HWFirstSegmentTimeout time.Duration

	// WaitBudgetDur is how long the segment handler blocks on cache.Wait for an
	// orchestrated segment (returned by WaitBudget()). Default 25 s — a generous
	// HW cold-start window. Configurable so unit tests fail fast instead of summing
	// 25 s waits, mirroring the software producer's WaitBudgetDur.
	WaitBudgetDur time.Duration

	// HWPerVariant routes the HW transcode path through one single-output ffmpeg
	// per ABR rung (forwarded to HLSProcessOpts.PerVariantHW). Off by default;
	// software is always single-process multi-variant.
	HWPerVariant bool

	// SingleVariant, when true, makes the orchestrator spawn one ffmpeg rafale
	// per (fingerprint, variantIdx) pair instead of one multi-variant rafale per
	// fingerprint. Each spawn produces only the requested ABR rung (1 encode),
	// saving the N-1 encodes of rungs the client never watches. Like
	// Jellyfin/Plex's on-demand single-variant model.
	//
	// FLAG OFF (default): behaviour is byte-identical to the pre-flag multi-variant
	// path — the existing tests prove this invariant.
	// FLAG ON: the per-rung sessions are all keyed under the same DecisionHash;
	// the client ABR-switches freely and each new rung is produced lazily.
	SingleVariant bool
}

func (o *OrchestratorOptions) withDefaults() OrchestratorOptions {
	out := *o
	if out.LookAheadSegments <= 0 {
		out.LookAheadSegments = 30
	}
	if out.InactivityTimeout <= 0 {
		out.InactivityTimeout = 60 * time.Second
	}
	if out.GCInterval <= 0 {
		out.GCInterval = 15 * time.Second
	}
	if out.SegmentSeconds <= 0 {
		out.SegmentSeconds = ManifestSegmentSeconds
	}
	if out.ThrottlePauseAhead <= 0 {
		out.ThrottlePauseAhead = 10
	}
	if out.ThrottleResumeBelow <= 0 {
		out.ThrottleResumeBelow = 5
	}
	if out.ThrottleResumeBelow >= out.ThrottlePauseAhead {
		// Hysteresis invariant: resume threshold must be strictly below pause
		// threshold or the session will flap every tick.
		out.ThrottleResumeBelow = out.ThrottlePauseAhead / 2
	}
	if out.ThrottleTickInterval <= 0 {
		out.ThrottleTickInterval = time.Second
	}
	if out.MaxRafalesPerHash <= 0 {
		out.MaxRafalesPerHash = 3
	}
	if out.HWFirstSegmentTimeout <= 0 {
		out.HWFirstSegmentTimeout = 5 * time.Second
	}
	if out.WaitBudgetDur <= 0 {
		out.WaitBudgetDur = 25 * time.Second
	}
	return out
}

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
	// calibration service can demote the host capability. Optional/nil-safe —
	// when unset, a HW failure just falls through to the normal 503-retry.
	hwDemote func(ctx context.Context, class transcoding.FailureClass, codec string)

	// shutdownCtx is the long-lived parent context for every ffmpeg session
	// the orchestrator spawns. Never the HTTP request context — that one is
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
	// pathological hosts — beyond the cap the LRU rafale is evicted.
	sessions map[string][]*orchestratorSession

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
	// cleanup pass — leaking tempdirs in tests and racing with the
	// orchestrator's lifecycle in prod.
	prewarmWG sync.WaitGroup

	// spawnedHashes is the set of every cache hash this orchestrator
	// has ever spawned a session for. We track it separately from the
	// live `sessions` map so Close() can cleanup the disk even when
	// sweepIdle has already deleted the session struct from the live
	// map — the disk segments stay on the host until an explicit
	// shutdown cleanup, and we don't want a goroutine ordering race
	// between sweepIdle and Close to leak them.
	//
	// KNOWN BOUNDED GROWTH: unlike the software producer (whose sweepIdle
	// reclaims the hash dir and can therefore also drop the set entry), the
	// orchestrator's disk is reclaimed by the cache's LRU eviction, which this
	// struct never observes — so entries here outlive their dir and accumulate
	// (~60 B per distinct hash) until Close. The Close-time CleanupHash on an
	// already-evicted hash is a harmless no-op, and the OOM horizon is years;
	// pruning would need an eviction callback from SegmentCache, not worth the
	// plumbing at this magnitude.
	spawnedHashes map[string]struct{}

	// sessionSem is a counting semaphore bounding the total number of live
	// ffmpeg rafales to opts.MaxActiveSessions. nil when uncapped
	// (MaxActiveSessions <= 0). A slot is acquired in spawnSession before ffmpeg
	// starts and released exactly once — by the reap goroutine when the session
	// exits, or by an early-return / spawn-race-lost path before the reap
	// goroutine takes ownership.
	sessionSem chan struct{}
}

// SourceResolver bridges the orchestrator (which works with DecisionFingerprint)
// to whatever the rest of the app uses to locate a media file and its streams.
// Kept as a small interface so the orchestrator stays testable without bringing
// the whole library service into its tests.
type SourceResolver interface {
	// ResolveSource returns the absolute file path, media streams (codec/lang/
	// resolution metadata used by the ffmpeg adapter), and a "needs HDR tonemap"
	// flag for the given media. Errors propagate to the handler as 5xx.
	ResolveSource(ctx context.Context, fp DecisionFingerprint) (path string, streams []SourceStream, err error)
}

// SourceResolverFunc adapts a plain function to the SourceResolver interface,
// letting wire.go inline the bridge without declaring a one-method struct.
type SourceResolverFunc func(ctx context.Context, fp DecisionFingerprint) (string, []SourceStream, error)

func (f SourceResolverFunc) ResolveSource(ctx context.Context, fp DecisionFingerprint) (string, []SourceStream, error) {
	return f(ctx, fp)
}

// SourceStream is the minimal stream metadata the orchestrator passes to the
// ffmpeg adapter. Defined here (rather than re-using library.MediaStream
// directly) to avoid pulling the library package into the orchestrator.
type SourceStream struct {
	Index         int
	Type          string // "video", "audio", "subtitle"
	Codec         string
	Language      string
	IsDefault     bool
	IsExternal    bool
	ExternalPath  string
	Width, Height int
	FrameRate     float64
	PixelFormat   string
}

type orchestratorSession struct {
	mu         sync.Mutex
	hash       string
	mediaID    string // for RemoveSession reverse lookup (a single mediaID can have several hashes — different audio/sub/codec decisions)
	handle     transcoding.HLSProcessHandle
	startSeg   int // first segment this rafale is producing
	endSeg     int // last segment (inclusive) the rafale will produce
	lastAccess time.Time
	teardown   func() // SegmentCache.RegisterSession's teardown
	cancelCtx  context.CancelFunc
	done       chan struct{} // closed when ffmpeg exits (success or error)

	// variantIdx is meaningful only when opts.SingleVariant is true. In that
	// mode each session encodes exactly one ABR rung and the session-match
	// logic in findCoveringSessionLocked uses it to keep per-rung sessions
	// isolated. When SingleVariant is false this field is 0 and unused.
	variantIdx int

	// Throttle state. clientHighWater is the largest segment index any client
	// has asked us to ensure for this session — used as a proxy for "how far
	// the client has consumed". paused tracks whether the ffmpeg process is
	// currently SIGSTOPped; useFMP4 is captured at spawn so the throttle loop
	// can query the cache without the fingerprint.
	clientHighWater int
	paused          bool
	useFMP4         bool
	// lastResumeAt rate-limits NotifyClientHint's SIGCONT path.
	// A malicious / buggy client could forge `bs=1` at 1000/sec and
	// keep us hammering the kernel with SIGCONT syscalls — idempotent
	// on a running process but wasted scheduler time. One Resume per
	// second per session is plenty (the player rebuffers more slowly
	// than that anyway).
	lastResumeAt time.Time

	// metricDecOnce guarantees the HLSVODSessions gauge is decremented EXACTLY
	// once per spawned session, whether it leaves via the reap goroutine (natural
	// ffmpeg exit — the dominant path when a viewer watches to the end) or via
	// killSession (GC / seek-cancel / watchdog). Without it the reap path never
	// decremented and the gauge climbed monotonically.
	metricDecOnce sync.Once
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
// Optional — left unset in tests / when no calibration service is present.
func (o *SegmentOrchestrator) SetHWDemoteSink(sink func(ctx context.Context, class transcoding.FailureClass, codec string)) {
	o.hwDemote = sink
}

// Close stops the inactivity GC, kills all live sessions, and purges their
// disk segments. cancel-on-seek and inactivity-kill intentionally leave the
// hash on disk so future requests can hit it warm; Close is the explicit
// shutdown path where we DO want the cache freed (the orchestrator is going
// away — nothing will hit those segments again). Safe to call once.
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

// findCoveringSessionLocked picks a live session whose [startSeg, endSeg]
// range covers segmentIdx (or whose nearAhead window does). Returns nil
// when no rafale serves this position — caller spawns a new one. Must
// be called with o.mu held.
//
// When opts.SingleVariant is true, only sessions whose variantIdx matches the
// requested variantIdx are eligible — each ABR rung has its own dedicated
// session. When SingleVariant is false, variantIdx is ignored and the first
// covering session for the hash is returned (multi-variant behaviour unchanged).
func (o *SegmentOrchestrator) findCoveringSessionLocked(hash string, variantIdx, segmentIdx, nearAheadBudget int) *orchestratorSession {
	for _, s := range o.sessions[hash] {
		// In single-variant mode each rung has its own session; skip sessions
		// that encode a different rung. In multi-variant mode every session
		// encodes all rungs, so the variantIdx filter is suppressed.
		if o.opts.SingleVariant && s.variantIdx != variantIdx {
			continue
		}
		s.mu.Lock()
		inWindow := segmentIdx >= s.startSeg && segmentIdx <= s.endSeg
		nearAhead := segmentIdx > s.endSeg && segmentIdx <= s.endSeg+nearAheadBudget
		s.mu.Unlock()
		if inWindow || nearAhead {
			return s
		}
	}
	return nil
}

// evictLRUIfFullLocked enforces MaxRafalesPerHash by killing the
// least-recently-touched rafale when the cap is reached. Run AFTER
// the new session has been appended to o.sessions[hash], so the
// freshly-spawned one is never the eviction candidate. Must be called
// with o.mu held.
//
// Returns the evicted session (or nil) so the caller can run killSession
// outside the lock — killSession itself reacquires o.mu.
func (o *SegmentOrchestrator) evictLRUIfFullLocked(hash string) *orchestratorSession {
	list := o.sessions[hash]
	if len(list) <= o.opts.MaxRafalesPerHash {
		return nil
	}
	// Find the LRU among the OLDER entries (skip the newly-appended one
	// at index len-1 — it has the freshest lastAccess by construction).
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
	// Splice the victim out of the list — killSession does the rest
	// (close ffmpeg, teardown cache pending channel) without holding
	// o.mu so we don't deadlock when it reacquires the lock at the
	// end of its body.
	o.sessions[hash] = append(list[:victimIdx], list[victimIdx+1:]...)
	return victim
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
func (o *SegmentOrchestrator) PreWarm(fp DecisionFingerprint, startSegIdx int) {
	// Skip cleanly if Close already ran. shutdownCtx.Err() is non-nil after
	// shutdownCancel — spawning a goroutine then would race the cache wipe.
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
		// Pre-warm always targets variant 0 (the base rung). In single-variant
		// mode only that rung is warmed; the player will fetch other rungs on
		// demand. In multi-variant mode this warms the whole ladder as before.
		if err := o.EnsureSegmentRange(ctx, fp, 0, startSegIdx); err != nil {
			o.logger.Debug("orchestrator: prewarm failed (non-fatal)",
				slog.String("media_id", fp.MediaID),
				slog.Any("err", err))
		}
	})
}

// EnsureSegmentRange guarantees that segment `segmentIdx` of `variantIdx` will
// be produced (or is already on disk). Returns immediately — callers must use
// cache.Wait to block on segment readiness. The returned error is non-nil only
// on terminal setup failure (resolver/source unavailable, ffmpeg spawn refused).
//
// variantIdx identifies the ABR rung the client is requesting. When
// SingleVariant is OFF (the default) this value is forwarded to the cache
// fast-path check but otherwise ignored for session-matching — the existing
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
	// does, spawn a fresh rafale ALONGSIDE the existing ones — we
	// don't tear them down anymore. The previous "kill on
	// out-of-window seek" policy was correct for a single-client
	// orchestrator but broke as soon as two clients sat on different
	// positions of the same media (the second seek killed the
	// first's rafale). Concurrency is bounded by MaxRafalesPerHash;
	// past the cap the LRU rafale is evicted by evictLRUIfFullLocked
	// after the new spawn.
	//
	// When SingleVariant is ON, findCoveringSessionLocked additionally
	// filters by variantIdx so per-rung sessions stay isolated.
	o.mu.Lock()
	session := o.findCoveringSessionLocked(hash, variantIdx, segmentIdx, o.opts.LookAheadSegments)
	o.mu.Unlock()

	if session != nil {
		session.mu.Lock()
		inWindow := segmentIdx >= session.startSeg && segmentIdx <= session.endSeg
		nearAhead := segmentIdx > session.endSeg && segmentIdx <= session.endSeg+o.opts.LookAheadSegments
		session.lastAccess = time.Now()
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

	// No rafale covers this position — spawn a new one. If we cross
	// MaxRafalesPerHash, the post-spawn evictLRUIfFullLocked pass
	// retires the LRU.
	o.logger.Info("orchestrator: spawning additional rafale for unserved seek",
		slog.String("hash", hash),
		slog.Int("requested", segmentIdx))
	return o.spawnSession(ctx, fp, variantIdx, segmentIdx)
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
	// ffmpeg — byte-identical to the pre-flag behaviour.
	var sessionProfiles []transcoding.HLSProfile
	var sessionVariantIdx int
	if o.opts.SingleVariant {
		if variantIdx < 0 || variantIdx >= len(fp.Profiles) {
			teardown()
			releaseSlot()
			return fmt.Errorf("spawnSession: variantIdx %d out of range (profiles=%d)", variantIdx, len(fp.Profiles))
		}
		sessionProfiles = profilesToDomain(fp.Profiles[variantIdx : variantIdx+1])
		sessionVariantIdx = variantIdx
	} else {
		sessionProfiles = profilesToDomain(fp.Profiles)
		sessionVariantIdx = 0 // multi-variant: VariantIndex unused by the multi-variant builder
	}

	procOpts := transcoding.HLSProcessOpts{
		OutputDir:          outDir,
		FilePath:           srcPath,
		Profiles:           sessionProfiles,
		VariantIndex:       sessionVariantIdx,
		VideoDecision:      "transcode",
		AudioDecision:      fp.AudioDecision(),
		AudioTrackIndex:    fp.AudioTrack,
		AudioUserOffsetSec: float64(fp.AudioOffsetMs) / 1000.0,
		SubtitleTrackIndex: fp.SubtitleBurnInTrack,
		HDRSupported:       false,         // tonemap is part of decision; HDRSupported=false on the client forces tonemap
		ToneMap:            fp.HDRTonemap, // HDR->SDR tonemap decision, carried as a bool (Streams not threaded here)
		AudioNormalize:     fp.AudioNormalize,
		StartSec:           float64(startSegIdx) * o.opts.SegmentSeconds,
		Streams:            nil, // burn-in still wants real streams (Phase 3); HDR tonemap is wired via ToneMap above
		VideoCodec:         fp.VideoCodec,
		AudioCodec:         fp.AudioCodec,
		MaxAudioChannels:   fp.MaxAudioChannels,
		ForceSidecarSubs:   fp.ForceSidecarSubs,
		// fp.UseFMP4 is the container choice baked into the manifest the
		// player will receive (.m4s vs .ts). Without forwarding it, ffmpeg
		// always wrote .ts via the multi-variant builder while the manifest
		// pointed at .m4s — segment fetches missed the cache forever.
		UseFMP4: fp.UseFMP4,
		// Hardware encode backend resolved by the handler (empty = software).
		// The ffmpeg builder switches to the zero-copy GPU pipeline only when
		// HWAccel is a valid non-None backend AND the content is simple.
		HWAccel:      transcoding.HWAccelType(fp.HWAccel),
		HWDevice:     fp.HWDevice,
		HWFeatures:   fp.HWFeatures,
		HWTonemap:    fp.HWTonemap, // GPU HDR->SDR fold (resolved by the handler; false -> software tonemap)
		PerVariantHW: o.opts.HWPerVariant,
	}

	sessionCtx, cancel := context.WithCancel(o.shutdownCtx)
	handle, err := o.process.Start(sessionCtx, procOpts)
	if err != nil {
		cancel()
		teardown()
		releaseSlot()
		return fmt.Errorf("spawnSession: ffmpeg start: %w", err)
	}

	session := &orchestratorSession{
		hash:            hash,
		mediaID:         fp.MediaID,
		handle:          handle,
		startSeg:        startSegIdx,
		endSeg:          endSegIdx,
		lastAccess:      time.Now(),
		teardown:        teardown,
		cancelCtx:       cancel,
		done:            make(chan struct{}),
		clientHighWater: startSegIdx,
		useFMP4:         fp.UseFMP4,
		variantIdx:      variantIdx,
	}

	o.mu.Lock()
	// Multi-rafale spawn race: only veto the new spawn if another
	// goroutine has ALREADY spawned a rafale that covers startSegIdx
	// (window or nearAhead). Multiple rafales on disjoint ranges of
	// the same hash are the whole point of this refactor — letting
	// them coexist is what fixes the multi-client / multi-tab bug.
	// In single-variant mode the race check also filters by variantIdx
	// so two goroutines requesting the same rung at the same position
	// are correctly deduplicated.
	if existing := o.findCoveringSessionLocked(hash, variantIdx, startSegIdx, o.opts.LookAheadSegments); existing != nil {
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
		return nil
	}
	o.sessions[hash] = append(o.sessions[hash], session)
	o.spawnedHashes[hash] = struct{}{}
	evicted := o.evictLRUIfFullLocked(hash)
	o.mu.Unlock()

	// LRU-evict the oldest rafale outside the lock — killSession
	// reacquires o.mu so we can't hold it here.
	if evicted != nil {
		o.logger.Info("orchestrator: LRU-evicting rafale (cap reached)",
			slog.String("hash", hash),
			slog.Int("evicted_start_seg", evicted.startSeg))
		o.killSession(evicted, "lru_cap")
	}

	// Reap when ffmpeg exits so the session entry doesn't linger
	// forever. Releases its cache-pending refcount via teardown too —
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
				slog.Error("segment_orchestrator: detached reap goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
			}
		}()
		// Distinguish a natural ffmpeg exit (inspect why) from our own teardown
		// (seek-cancel / GC / shutdown via sessionCtx) — only the former can be
		// a hardware-capability failure worth demoting on.
		naturalExit := false
		select {
		case <-handle.Done():
			naturalExit = true
		case <-sessionCtx.Done():
		}
		close(session.done)
		session.mu.Lock()
		teardown := session.teardown
		session.teardown = nil
		session.mu.Unlock()
		if teardown != nil {
			teardown()
		}
		o.removeSessionFromList(hash, session)
		// Natural exit: decrement the live-sessions gauge (exactly once — see
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

		// Level-2 safety net: a HW-capability failure demotes the host
		// capability so the client's automatic retry re-resolves a lower tier
		// (the cache is encoder-agnostic, so the fallback segments slot into the
		// same manifest). Cancellation / transient / unknown are ignored.
		if naturalExit && o.hwDemote != nil {
			class := handle.Failure()
			usedHW := procOpts.HWAccel != ""
			switch {
			case usedHW && class.IsHWCapability():
				o.logger.Warn("orchestrator: HW encode failure, demoting capability",
					slog.String("hash", hash),
					slog.String("class", string(class)),
					slog.String("codec", fp.VideoCodec))
				o.hwDemote(o.shutdownCtx, class, fp.VideoCodec)

			case usedHW && class != transcoding.FailureCancelled && !o.hashProducedSegment(hash, variantIdx, fp.UseFMP4):
				// A HW session that exited on its own without writing a single
				// segment is a silent HW death the stderr classifier can't see:
				// a driver crash (SIGSEGV inside the GPU .so / a TDR GPU reset on
				// WSL d3d12) or an exit-0 "Nothing was written" (e.g. a 10-bit
				// source fed to an 8-bit encoder). Either way the encoder isn't
				// delivering here, so demote to software (hw_init ⇒ disable HW +
				// persist) and let the client's retry play on CPU instead of
				// looping the same broken HW path to the patience timeout.
				o.logger.Warn("orchestrator: HW session produced no segments, demoting to software",
					slog.String("hash", hash),
					slog.String("class", string(class)),
					slog.String("hw_backend", string(procOpts.HWAccel)),
					slog.String("codec", fp.VideoCodec))
				o.hwDemote(o.shutdownCtx, transcoding.FailureHWInit, fp.VideoCodec)
			}
		}

		// Free the global concurrency slot: this session has fully exited and is
		// off the live list, so a queued spawn may now proceed.
		o.releaseSessionSlot()
	}()

	// HW fast-fail watchdog: a hardware rafale that hasn't written its init
	// segment within HWFirstSegmentTimeout is a stuck/dead GPU pipeline (a TDR
	// reset, a hung encoder). Don't make the client wait out the full ~25 s
	// segment timeout — demote to software now and kill the session so the
	// retry re-resolves the (encoder-agnostic) software tier. The init is the
	// signal, not a full segment: a working-but-slow GPU writes it within
	// ~1-3 s while a dead one never does, so the tight window doesn't false-
	// fail a slow weak-NAS encoder. Only HW sessions, only when a demote sink
	// is wired (otherwise a kill would just re-spawn the same broken HW).
	if procOpts.HWAccel != "" && o.opts.HWFirstSegmentTimeout > 0 && o.hwDemote != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("segment_orchestrator: detached HW watchdog goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
				}
			}()
			t := time.NewTimer(o.opts.HWFirstSegmentTimeout)
			defer t.Stop()
			select {
			case <-session.done: // already exited — the reap goroutine handles it
				return
			case <-sessionCtx.Done(): // cancelled (seek / GC / shutdown)
				return
			case <-t.C:
			}
			if o.hwSessionAlive(hash, variantIdx, fp.UseFMP4) {
				return // the encoder is producing (maybe slowly) — let it run
			}
			o.logger.Warn("orchestrator: HW session wrote no init within watchdog window, demoting to software",
				slog.String("hash", hash),
				slog.String("hw_backend", string(procOpts.HWAccel)),
				slog.String("codec", fp.VideoCodec),
				slog.Duration("window", o.opts.HWFirstSegmentTimeout))
			o.hwDemote(o.shutdownCtx, transcoding.FailureHWInit, fp.VideoCodec)
			o.killSession(session, "hw watchdog: no init segment")
		}()
	}

	o.logger.Info("orchestrator: rafale spawned",
		slog.String("hash", hash),
		slog.Int("start_seg", startSegIdx),
		slog.Int("end_seg", endSegIdx),
		slog.String("source", srcPath))

	metrics.HLSVODRafaleSpawned.WithLabelValues("miss").Inc()
	metrics.HLSVODSessions.Inc()

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
// variantIdx selects which rung's init/segment to check — in multi-variant mode
// this is always 0 (base rung); in single-variant mode it is the session's own rung.
func (o *SegmentOrchestrator) hwSessionAlive(hash string, variantIdx int, useFMP4 bool) bool {
	if o.cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: variantIdx, IsInit: true}, useFMP4) {
		return true
	}
	return o.hashProducedSegment(hash, variantIdx, useFMP4)
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
	// which also wakes the reap — only the first of the two decrements the gauge.
	s.metricDecOnce.Do(func() { metrics.HLSVODSessions.Dec() })
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

// touchSession bumps lastAccess + client high-water on the rafale that
// covers segmentIdx, falling back to the most-recently-accessed
// rafale of the hash when no exact cover exists (cache-hit case: the
// client is fetching from disk, but we still want to keep at least
// one rafale warm for the upcoming segments). Used on cache hits so
// the GC doesn't reap a session whose segments are still being
// consumed from the cache, and so the throttle loop sees client
// progress and resumes a paused ffmpeg in time.
//
// variantIdx is forwarded to findCoveringSessionLocked so that in
// single-variant mode the right per-rung session is touched.
func (o *SegmentOrchestrator) touchSession(hash string, variantIdx, segmentIdx int) {
	o.mu.Lock()
	target := o.findCoveringSessionLocked(hash, variantIdx, segmentIdx, o.opts.LookAheadSegments)
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
	if segmentIdx > target.clientHighWater {
		target.clientHighWater = segmentIdx
	}
	target.mu.Unlock()
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
			o.sweepIdle()
		}
	}
}

func (o *SegmentOrchestrator) sweepIdle() {
	cutoff := time.Now().Add(-o.opts.InactivityTimeout)
	o.mu.Lock()
	var toKill []*orchestratorSession
	for _, list := range o.sessions {
		for _, s := range list {
			s.mu.Lock()
			idle := s.lastAccess.Before(cutoff)
			s.mu.Unlock()
			if idle {
				toKill = append(toKill, s)
			}
		}
	}
	o.mu.Unlock()
	for _, s := range toKill {
		o.killSession(s, "inactivity")
	}
}

// throttleLoop applies pause-when-ahead to every live session on a ticker.
// Pattern lifted from Plex's transcode pipeline (audit competitors_big): when
// ffmpeg has produced ThrottlePauseAhead segments more than the client's
// high-water, SIGSTOP it — we burn 0 CPU until the client catches up.
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
			o.evaluateThrottle()
		}
	}
}

// evaluateThrottle runs one tick of the pause-when-ahead policy. For each
// live session, ListReady gives the latest produced segment idx; the buffer
// (produced - client) decides pause vs resume.
func (o *SegmentOrchestrator) evaluateThrottle() {
	o.mu.Lock()
	var snapshot []*orchestratorSession
	for _, list := range o.sessions {
		snapshot = append(snapshot, list...)
	}
	o.mu.Unlock()

	for _, s := range snapshot {
		s.mu.Lock()
		hash := s.hash
		client := s.clientHighWater
		paused := s.paused
		handle := s.handle
		useFMP4 := s.useFMP4
		variantIdx := s.variantIdx
		s.mu.Unlock()

		if handle == nil {
			continue
		}

		// Track this session's own rung: in SingleVariant mode a rung>0 session
		// writes stream_<variantIdx>_*, so listing variant 0 would always find
		// nothing and never throttle it (CPU/disk waste). In the default
		// multi-variant mode stream_0 tracks the shared session, so variantIdx==0.
		idxs, err := o.cache.ListReady(hash, variantIdx, useFMP4)
		if err != nil || len(idxs) == 0 {
			continue
		}
		produced := idxs[len(idxs)-1]
		buffer := produced - client

		switch {
		case !paused && buffer >= o.opts.ThrottlePauseAhead:
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
				slog.Int("buffer", buffer))
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
// client is actually experiencing — buffer length, imminent starvation,
// real cursor — instead of the coarse "last requested segment" proxy.
//
// CRITICAL constraint: this method NEVER changes which variant the client
// downloads. The HLS protocol gives the player full control over rendition
// selection (the segment URL carries the variant index). All we can do
// here is adjust the encoder's pace and lookahead, and wake up a paused
// ffmpeg — all transparent to the client and to the user's manual quality
// lock.
//
// Effects per session whose mediaID matches:
//
//   - clientHighWater bumped forward when the CMCD-derived cursor is
//     ahead of what touchSession had previously seen. We only ever bump
//     forward — a stale CMCD packet must not pull the throttle back.
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
func (o *SegmentOrchestrator) NotifyClientHint(mediaItemID string, bufferLengthMs int, bufferStarved bool, segmentIdx int) {
	o.mu.Lock()
	matched := make([]*orchestratorSession, 0, 1)
	for _, list := range o.sessions {
		for _, s := range list {
			s.mu.Lock()
			mid := s.mediaID
			s.mu.Unlock()
			if mid == mediaItemID {
				matched = append(matched, s)
			}
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
	// Cap the prefetch reach at 2× the rafale lookahead — combined with the
	// duration clamp on consumableIdx, endSeg can never run away from the real
	// playback position.
	if maxAhead := 2 * o.opts.LookAheadSegments; bufferAheadSegs > maxAhead {
		bufferAheadSegs = maxAhead
	}
	consumableIdx := max(segmentIdx, 0)

	now := time.Now()
	for _, s := range matched {
		s.mu.Lock()
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

// ActiveCount returns how many ffmpeg sessions are currently live across
// every fingerprint. Test + observability accessor; cheap to call.
// Counts all rafales in the slice, not just distinct hashes — a film
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
// admin/system metrics gauge consume — same shape as the legacy HLSManager's
// counter, so the trickplay ActiveStreamChecker contract drops in without
// changing callers.
func (o *SegmentOrchestrator) ActiveSessionCount() int { return o.ActiveCount() }

// RemoveSession kills every rafale currently producing for the given media
// ID. A single media can have several live sessions when different clients
// negotiate different audio tracks / sub burn-ins / codecs (one DecisionHash
// each), so the legacy contract — keyed by mediaID — fans out here. Safe to
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

// CleanupAll mirrors the legacy HLSManager.CleanupAll — deferred from main
// at shutdown. Same observable behaviour as Close (kill every ffmpeg + free
// every hash on disk) but kept as a distinct method so the wiring side can
// `defer` it without ambiguity.
func (o *SegmentOrchestrator) CleanupAll() { o.Close() }

// Compile-time assertion: the orchestrator IS the hardware StreamProducer.
// It already carried EnsureSegmentRange / PreWarm / NotifyClientHint /
// ActiveSessionCount / RemoveSession / Close; Profiles + WaitBudget below
// complete the interface. The behaviour of every pre-existing method is
// unchanged — this is the HW impl behind the seam.
var _ StreamProducer = (*SegmentOrchestrator)(nil)

// WaitBudget is how long the segment handler should block on cache.Wait for an
// orchestrated (hardware) segment. The HW path bursts ahead and lands segment
// 0 within a few seconds on a working GPU, so 25 s is a generous worst-case
// cold-start window (it matches the handler's historical segmentWaitTimeout).
func (o *SegmentOrchestrator) WaitBudget() time.Duration { return o.opts.WaitBudgetDur }

// Profiles shapes the ABR ladder for the HARDWARE path: the full ladder (a GPU
// parallelises rungs cheaply) capped to the source resolution and the host's
// calibrated HW ceiling. Pure in its inputs — the same (base, src, capacity)
// yields the same ladder at manifest-time and segment-time, the cache-hash
// invariant. decision is unused here (the HW path keeps every rung); it is
// part of the interface so the software producer can branch on HDR.
func (o *SegmentOrchestrator) Profiles(base []transcoding.HLSProfile, src SourceDims, _ transcoding.PlaybackDecision, capacity transcoding.EncodeCapability) []transcoding.HLSProfile {
	sourceCapped := transcoding.CapLadderToSource(base, src.Width, src.Height)
	return transcoding.CapLadderToHeight(sourceCapped, capacity.MaxHeightFor(true))
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
