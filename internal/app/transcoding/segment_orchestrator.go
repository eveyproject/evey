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
	}
	go o.gcLoop()
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
	o.shutdownCancel()
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
		o.touchSession(hash)
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
	}

	sessionCtx, cancel := context.WithCancel(o.shutdownCtx)
	handle, err := o.process.Start(sessionCtx, procOpts)
	if err != nil {
		cancel()
		teardown()
		return fmt.Errorf("spawnSession: ffmpeg start: %w", err)
	}

	session := &orchestratorSession{
		hash:       hash,
		mediaID:    fp.MediaID,
		handle:     handle,
		startSeg:   startSegIdx,
		endSeg:     endSegIdx,
		lastAccess: time.Now(),
		teardown:   teardown,
		cancelCtx:  cancel,
		done:       make(chan struct{}),
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

// touchSession bumps lastAccess without holding the orchestrator lock for the
// whole touch. Used on cache hits so the GC doesn't reap a session whose
// segments are still being consumed from the cache.
func (o *SegmentOrchestrator) touchSession(hash string) {
	o.mu.Lock()
	s := o.sessions[hash]
	o.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastAccess = time.Now()
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

