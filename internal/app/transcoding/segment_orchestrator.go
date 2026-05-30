package transcoding

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

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
	cache    *SegmentCache
	process  transcoding.HLSProcess
	source   SourceResolver
	opts     OrchestratorOptions
	logger   *slog.Logger

	// shutdownCtx is the long-lived parent context for every ffmpeg session
	// the orchestrator spawns. Never the HTTP request context — that one is
	// cancelled the moment the segment is served, which would tear down the
	// rafale before the next segment is requested. Cancelled only by Close.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	mu       sync.Mutex
	sessions map[string]*orchestratorSession

	stopGC chan struct{}
	gcDone chan struct{}

	stopThrottle chan struct{}
	throttleDone chan struct{}

	// prewarmWG tracks PreWarm goroutines so Close() can drain them before
	// nuking the cache. Without this, a goroutine spawned by serveVariant
	// could write fresh files into the cache dir AFTER Close() ran its
	// cleanup pass — leaking tempdirs in tests and racing with the
	// orchestrator's lifecycle in prod.
	prewarmWG sync.WaitGroup
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

	// Throttle state. clientHighWater is the largest segment index any client
	// has asked us to ensure for this session — used as a proxy for "how far
	// the client has consumed". paused tracks whether the ffmpeg process is
	// currently SIGSTOPped; useFMP4 is captured at spawn so the throttle loop
	// can query the cache without the fingerprint.
	clientHighWater int
	paused          bool
	useFMP4         bool
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
		sessions:       make(map[string]*orchestratorSession),
		stopGC:         make(chan struct{}),
		gcDone:         make(chan struct{}),
		stopThrottle:   make(chan struct{}),
		throttleDone:   make(chan struct{}),
	}
	go o.gcLoop()
	if o.opts.ThrottleEnabled {
		go o.throttleLoop()
	} else {
		close(o.throttleDone)
	}
	return o
}

// Close stops the inactivity GC, kills all live sessions, and purges their
// disk segments. cancel-on-seek and inactivity-kill intentionally leave the
// hash on disk so future requests can hit it warm; Close is the explicit
// shutdown path where we DO want the cache freed (the orchestrator is going
// away — nothing will hit those segments again). Safe to call once.
func (o *SegmentOrchestrator) Close() {
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

	o.mu.Lock()
	sessions := make([]*orchestratorSession, 0, len(o.sessions))
	for _, s := range o.sessions {
		sessions = append(sessions, s)
	}
	o.sessions = make(map[string]*orchestratorSession)
	o.mu.Unlock()

	for _, s := range sessions {
		o.killSession(s, "close")
		if err := o.cache.CleanupHash(s.hash); err != nil {
			o.logger.Warn("orchestrator: close cleanup failed",
				slog.String("hash", s.hash),
				slog.Any("err", err))
		}
	}
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
	o.prewarmWG.Add(1)
	go func() {
		defer o.prewarmWG.Done()
		ctx, cancel := context.WithTimeout(o.shutdownCtx, 30*time.Second)
		defer cancel()
		if err := o.EnsureSegmentRange(ctx, fp, startSegIdx); err != nil {
			o.logger.Debug("orchestrator: prewarm failed (non-fatal)",
				slog.String("media_id", fp.MediaID),
				slog.Any("err", err))
		}
	}()
}

// EnsureSegmentRange guarantees that segment `segmentIdx` will be produced (or
// is already on disk). Returns immediately — callers must use cache.Wait to
// block on segment readiness. The returned error is non-nil only on terminal
// setup failure (resolver/source unavailable, ffmpeg spawn refused).
//
// The orchestrator may spawn a fresh ffmpeg session, kill+respawn an existing
// one (cancel-on-seek), or short-circuit when the segment is already covered
// by a live session or is already in the cache.
func (o *SegmentOrchestrator) EnsureSegmentRange(
	ctx context.Context,
	fp DecisionFingerprint,
	segmentIdx int,
) error {
	if segmentIdx < 0 {
		return fmt.Errorf("EnsureSegmentRange: negative segmentIdx %d", segmentIdx)
	}
	hash := fp.Hash()

	// Fast path: segment already on disk (cache hit, no ffmpeg needed).
	// We don't know the variantIdx here — we only ensure the *first* variant
	// for the rafale; per-variant readiness is checked by the handler before
	// calling Wait. The cache check below is best-effort early bail.
	if o.cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: segmentIdx}, fp.UseFMP4) {
		o.touchSession(hash, segmentIdx)
		metrics.HLSVODCacheHits.Inc()
		return nil
	}
	metrics.HLSVODCacheMisses.Inc()

	o.mu.Lock()
	session, exists := o.sessions[hash]
	o.mu.Unlock()

	if exists {
		session.mu.Lock()
		inWindow := segmentIdx >= session.startSeg && segmentIdx <= session.endSeg
		// nearAhead: just past endSeg but within "ffmpeg will reach this
		// naturally" range. ffmpeg has no -t cap and runs to media EOF,
		// so seg endSeg+1 is already being encoded by the same process.
		// Extending the window keeps the same rafale and bumps lastAccess
		// so the inactivity GC doesn't reap a session that's still being
		// linearly consumed. Without this guard, every LookAheadSegments
		// played triggers a spurious cancel-on-seek + respawn — the exact
		// pathology that surfaced on the dev stress test.
		nearAhead := segmentIdx > session.endSeg && segmentIdx <= session.endSeg+o.opts.LookAheadSegments
		session.mu.Unlock()

		if inWindow || nearAhead {
			session.mu.Lock()
			session.lastAccess = time.Now()
			if segmentIdx > session.clientHighWater {
				session.clientHighWater = segmentIdx
			}
			if nearAhead {
				session.endSeg = segmentIdx + o.opts.LookAheadSegments
			}
			session.mu.Unlock()
			return nil
		}

		// True cancel-on-seek: segment far ahead OR rewound before window.
		// Tear down and respawn at the new offset. Don't hold o.mu while
		// killing — the ffmpeg Stop may block briefly on Wait().
		o.logger.Info("orchestrator: cancel-on-seek",
			slog.String("hash", hash),
			slog.Int("requested", segmentIdx),
			slog.Int("active_start", session.startSeg),
			slog.Int("active_end", session.endSeg))
		o.killSession(session, "seek_out_of_window")
	}

	return o.spawnSession(ctx, fp, segmentIdx)
}

// spawnSession launches an ffmpeg rafale for the given fingerprint at
// segmentIdx, producing up to startSeg + LookAheadSegments segments. Called
// under no lock; takes o.mu briefly to register the session.
func (o *SegmentOrchestrator) spawnSession(
	ctx context.Context,
	fp DecisionFingerprint,
	startSegIdx int,
) error {
	hash := fp.Hash()
	endSegIdx := startSegIdx + o.opts.LookAheadSegments - 1

	// Resolve source through the request context (auth + db calls live there)
	// but spawn ffmpeg under the orchestrator's long-lived context so it
	// survives past the original request returning to the client.
	srcPath, _, err := o.source.ResolveSource(ctx, fp)
	if err != nil {
		return fmt.Errorf("spawnSession: resolve source: %w", err)
	}
	// Phase 3 will wire real streams (burn-in / HDR tonemap); for Phase 1 the
	// ffmpeg adapter's defaults are sufficient for video + audio.

	outDir, err := o.cache.DirFor(hash)
	if err != nil {
		return fmt.Errorf("spawnSession: cache dir: %w", err)
	}

	teardown := o.cache.RegisterSession(hash)

	procOpts := transcoding.HLSProcessOpts{
		OutputDir:           outDir,
		FilePath:            srcPath,
		Profiles:            profilesToDomain(fp.Profiles),
		VideoDecision:       "transcode",
		AudioDecision:       "transcode",
		AudioTrackIndex:     fp.AudioTrack,
		SubtitleTrackIndex:  fp.SubtitleBurnInTrack,
		HDRSupported:        false, // tonemap is part of decision; HDRSupported=false on the client forces tonemap
		AudioNormalize:      fp.AudioNormalize,
		StartSec:            float64(startSegIdx) * o.opts.SegmentSeconds,
		Streams:             nil, // Phase 3 will wire real streams for burn-in/HDR tonemap; Phase 1 falls back on ffmpeg adapter defaults
		VideoCodec:          fp.VideoCodec,
		AudioCodec:          fp.AudioCodec,
		MaxAudioChannels:    fp.MaxAudioChannels,
		ForceSidecarSubs:    fp.ForceSidecarSubs,
		// fp.UseFMP4 is the container choice baked into the manifest the
		// player will receive (.m4s vs .ts). Without forwarding it, ffmpeg
		// always wrote .ts via the multi-variant builder while the manifest
		// pointed at .m4s — segment fetches missed the cache forever.
		UseFMP4: fp.UseFMP4,
	}

	sessionCtx, cancel := context.WithCancel(o.shutdownCtx)
	handle, err := o.process.Start(sessionCtx, procOpts)
	if err != nil {
		cancel()
		teardown()
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
	}

	o.mu.Lock()
	if existing, ok := o.sessions[hash]; ok {
		// Lost a race: another goroutine spawned for the same hash between our
		// check and now. Kill the loser (ourselves), keep the existing one. The
		// existing one's window may or may not include startSegIdx — that's the
		// caller's problem on the next call; here we just avoid two ffmpegs.
		o.mu.Unlock()
		o.logger.Info("orchestrator: spawn race lost, keeping existing session", slog.String("hash", hash))
		cancel()
		_ = handle.Stop()
		teardown()
		_ = existing // intentional no-op, keeps existing
		return nil
	}
	o.sessions[hash] = session
	o.mu.Unlock()

	// Reap when ffmpeg exits so the session entry doesn't linger forever.
	go func() {
		select {
		case <-handle.Done():
		case <-sessionCtx.Done():
		}
		close(session.done)
		o.mu.Lock()
		if cur, ok := o.sessions[hash]; ok && cur == session {
			delete(o.sessions, hash)
		}
		o.mu.Unlock()
	}()

	o.logger.Info("orchestrator: rafale spawned",
		slog.String("hash", hash),
		slog.Int("start_seg", startSegIdx),
		slog.Int("end_seg", endSegIdx),
		slog.String("source", srcPath))

	metrics.HLSVODRafaleSpawned.WithLabelValues("miss").Inc()
	metrics.HLSVODSessions.Inc()

	return nil
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

	o.mu.Lock()
	if cur, ok := o.sessions[hash]; ok && cur == s {
		delete(o.sessions, hash)
	}
	o.mu.Unlock()

	o.logger.Info("orchestrator: session killed",
		slog.String("hash", hash),
		slog.String("reason", reason))

	metrics.HLSVODSessionsKilled.WithLabelValues(reason).Inc()
	metrics.HLSVODSessions.Dec()
}

// touchSession bumps lastAccess and the client high-water without holding the
// orchestrator lock for the whole touch. Used on cache hits so the GC doesn't
// reap a session whose segments are still being consumed from the cache, and
// so the throttle loop sees client progress and resumes a paused ffmpeg in
// time.
func (o *SegmentOrchestrator) touchSession(hash string, segmentIdx int) {
	o.mu.Lock()
	s := o.sessions[hash]
	o.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastAccess = time.Now()
	if segmentIdx > s.clientHighWater {
		s.clientHighWater = segmentIdx
	}
	s.mu.Unlock()
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
	for _, s := range o.sessions {
		s.mu.Lock()
		idle := s.lastAccess.Before(cutoff)
		s.mu.Unlock()
		if idle {
			toKill = append(toKill, s)
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
	snapshot := make([]*orchestratorSession, 0, len(o.sessions))
	for _, s := range o.sessions {
		snapshot = append(snapshot, s)
	}
	o.mu.Unlock()

	for _, s := range snapshot {
		s.mu.Lock()
		hash := s.hash
		client := s.clientHighWater
		paused := s.paused
		handle := s.handle
		useFMP4 := s.useFMP4
		s.mu.Unlock()

		if handle == nil {
			continue
		}

		idxs, err := o.cache.ListReady(hash, 0, useFMP4)
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
	for _, s := range o.sessions {
		s.mu.Lock()
		mid := s.mediaID
		s.mu.Unlock()
		if mid == mediaItemID {
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
	consumableIdx := segmentIdx
	if consumableIdx < 0 {
		consumableIdx = 0
	}

	for _, s := range matched {
		s.mu.Lock()
		if consumableIdx > s.clientHighWater {
			s.clientHighWater = consumableIdx
		}
		needsResume := bufferStarved && s.paused
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
			s.mu.Unlock()
			o.logger.Info("orchestrator: throttle resumed by CMCD bs=1",
				slog.String("media_id", mediaItemID),
				slog.Int("segment_idx", segmentIdx))
			metrics.HLSVODThrottleResume.Inc()
			metrics.HLSVODStarvationResumes.Inc()
		}
	}
}

// ActiveCount returns how many ffmpeg sessions are currently live. Test +
// observability accessor; cheap to call.
func (o *SegmentOrchestrator) ActiveCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.sessions)
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
	for _, s := range o.sessions {
		s.mu.Lock()
		match := s.mediaID == mediaItemID
		s.mu.Unlock()
		if match {
			toKill = append(toKill, s)
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

