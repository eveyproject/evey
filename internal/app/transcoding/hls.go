package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

func IsImageSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub":
		return true
	}
	return false
}

type HLSSession struct {
	ID                 string
	MediaItemID        shared.MediaItemID
	UserID             shared.UserID
	OutputDir          string
	Profiles           []transcoding.HLSProfile
	VideoDecision      string // "copy" or "transcode"
	AudioDecision      string // "copy" or "transcode"
	AudioTrackIndex    int    // ffmpeg audio stream index (0:a:N)
	SubtitleTrackIndex int    // -1 = no subtitle
	SubtitleCodec      string
	HDRSupported       bool
	AudioNormalize     bool
	StartSec           float64 // seek offset
	// Overrides plumbed to the ffmpeg adapter. Zero values preserve legacy defaults.
	VideoCodec       string
	AudioCodec       string
	MaxAudioChannels int
	ForceSidecarSubs bool
	streams          []*library.MediaStream
	StartedAt        time.Time
	LastAccess       time.Time
	LastUserAgent    string // captured on each MarkAccess, "" until first handler call
	warnedIdle       bool   // gated so the "approaching idle kill" log fires at most once per session
	paused           bool   // ffmpeg SIGSTOPped by the flow-control loop (client idle)
	mu               sync.Mutex
	handle           transcoding.HLSProcessHandle // nil when serving cached transcode
	ready            chan struct{}
	done             chan struct{}
	err              error
}

// HLSOverrides carries decision-derived overrides (video codec, audio codec,
// channel cap, sidecar subtitle flag) into session creation. Zero values mean
// "use legacy defaults".
type HLSOverrides struct {
	VideoCodec       string
	AudioCodec       string
	MaxAudioChannels int
	ForceSidecarSubs bool
}

// HLSOption mutates session construction. Currently only WithOverrides is
// provided; kept as a functional option to avoid breaking existing callers.
type HLSOption func(*HLSSession)

func WithOverrides(o HLSOverrides) HLSOption {
	return func(s *HLSSession) {
		s.VideoCodec = o.VideoCodec
		s.AudioCodec = o.AudioCodec
		s.MaxAudioChannels = o.MaxAudioChannels
		s.ForceSidecarSubs = o.ForceSidecarSubs
	}
}

func (s *HLSSession) IsSingleStream() bool {
	return s.VideoDecision == "copy"
}

// MarkAccess refreshes LastAccess and optionally captures the caller's
// user-agent. Internal callers pass "" (no UA context); the HTTP segment
// handler passes r.UserAgent() so forensic logs on session death know
// which client was last on the line (Chromecast UAs start with "CrKey/").
func (s *HLSSession) MarkAccess(userAgent string) {
	s.mu.Lock()
	s.LastAccess = time.Now()
	if userAgent != "" {
		s.LastUserAgent = userAgent
	}
	s.warnedIdle = false
	resume := s.paused
	s.paused = false
	h := s.handle
	s.mu.Unlock()
	// Client is pulling again — let ffmpeg continue if the flow-control loop
	// had suspended it. Done outside the lock (h.Resume takes the handle's own
	// mutex); only fires on the paused→active edge so the hot segment path
	// isn't signalling ffmpeg on every request.
	if resume && h != nil {
		if err := h.Resume(); err != nil {
			slog.Warn("hls: resume ffmpeg failed",
				slog.String("media_id", string(s.MediaItemID)), slog.Any("err", err))
		}
	}
}

// pauseIfIdle suspends ffmpeg (SIGSTOP) when the client hasn't fetched a
// segment for longer than idle. ffmpeg's -readrate caps it at 1.5× real-time
// but does NOT tie it to the playhead, so a paused client would otherwise let
// ffmpeg keep encoding far past the paused position — burning CPU and disk on
// segments the user may never reach. The event-mode playlist itself is safe
// (no eviction), this is purely a host-resource guard. Returns true on the
// active→paused edge (for logging). No-op for cached sessions (nil handle).
func (s *HLSSession) pauseIfIdle(idle time.Duration, now time.Time) bool {
	s.mu.Lock()
	if s.paused || s.handle == nil || now.Sub(s.LastAccess) < idle {
		s.mu.Unlock()
		return false
	}
	s.paused = true
	h := s.handle
	s.mu.Unlock()
	if err := h.Pause(); err != nil {
		// Couldn't suspend (process already gone) — roll back so MarkAccess
		// doesn't try to resume a process that was never paused.
		s.mu.Lock()
		s.paused = false
		s.mu.Unlock()
		slog.Debug("hls: pause ffmpeg failed",
			slog.String("media_id", string(s.MediaItemID)), slog.Any("err", err))
		return false
	}
	return true
}

func (s *HLSSession) streamCount() int {
	if s.IsSingleStream() {
		return 1
	}
	return len(s.Profiles)
}

func (s *HLSSession) WaitReady(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.HLSSession.WaitReady")
	defer span.End()
	span.SetAttributes(
		attribute.String("session.id", s.ID),
		attribute.String("media_item.id", string(s.MediaItemID)),
	)
	select {
	case <-s.ready:
		if s.err != nil {
			span.RecordError(s.err)
			span.SetStatus(otelcodes.Error, s.err.Error())
			slog.Warn("hls: WaitReady — session start failed",
				slog.String("media_id", string(s.MediaItemID)),
				slog.String("user_id", string(s.UserID)),
				slog.Any("err", s.err))
		}
		return s.err
	case <-ctx.Done():
		err := ctx.Err()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.Warn("hls: WaitReady — ctx cancelled before session ready",
			slog.String("media_id", string(s.MediaItemID)),
			slog.String("user_id", string(s.UserID)),
			slog.Any("err", err))
		return err
	}
}

func (s *HLSSession) stop() {
	s.mu.Lock()
	h := s.handle
	ua := s.LastUserAgent
	uptime := time.Since(s.StartedAt)
	s.mu.Unlock()
	slog.Info("hls: session stopping",
		slog.String("media_id", string(s.MediaItemID)),
		slog.String("user_id", string(s.UserID)),
		slog.String("output_dir", s.OutputDir),
		slog.String("last_user_agent", ua),
		slog.Float64("uptime_seconds", uptime.Seconds()))
	if h != nil {
		if err := h.Stop(); err != nil {
			slog.Warn("hls: stop handle failed", slog.String("media_id", string(s.MediaItemID)), slog.Any("err", err))
		}
	}
	// Wait for startSession goroutine to finish before removing files
	<-s.done
	if err := os.RemoveAll(s.OutputDir); err != nil {
		slog.Warn("hls: remove output dir failed", slog.String("dir", s.OutputDir), slog.Any("err", err))
	}
}

type HLSManager struct {
	mu            sync.Mutex
	sessions      map[string]*HLSSession
	userSessions  map[string]int
	cacheDir      string
	maxConcurrent int
	sem           chan struct{} // buffered semaphore for concurrency control
	queueTimeout  time.Duration
	shutdownCtx   context.Context
	stopCleanup   chan struct{}
	cleanupOnce   sync.Once
	process       transcoding.HLSProcess
}

func NewHLSManager(shutdownCtx context.Context, cacheDir string, maxConcurrent int, process transcoding.HLSProcess) *HLSManager {
	var sem chan struct{}
	if maxConcurrent > 0 {
		sem = make(chan struct{}, maxConcurrent)
	}
	m := &HLSManager{
		sessions:      make(map[string]*HLSSession),
		userSessions:  make(map[string]int),
		cacheDir:      cacheDir,
		maxConcurrent: maxConcurrent,
		sem:           sem,
		queueTimeout:  10 * time.Second,
		shutdownCtx:   shutdownCtx,
		stopCleanup:   make(chan struct{}),
		process:       process,
	}
	sweepOrphanCacheDirs(cacheDir)
	go m.cleanupLoop()
	go m.pauseLoop()
	return m
}

const (
	// hlsPauseCheckInterval is how often the flow-control loop scans sessions
	// for idleness. Short so ffmpeg is suspended promptly after the client
	// stops pulling (no point transcoding ahead of a paused player).
	hlsPauseCheckInterval = 5 * time.Second
	// hlsPauseIdleThreshold is how long with no segment fetch before ffmpeg is
	// SIGSTOPped. Comfortably larger than hls.js's inter-fetch gap during
	// steady playback (it refills toward maxBufferLength≈30s by pulling a
	// segment every few seconds), so active playback never pauses; only a real
	// stall/pause (client buffer full, no fetches) trips it.
	hlsPauseIdleThreshold = 20 * time.Second
)

// pauseLoop suspends ffmpeg for sessions whose client has gone idle, and lets
// MarkAccess (segment/playlist fetch) resume them. This keeps the sliding
// window aligned with the playhead so a long pause can't slide it away.
func (m *HLSManager) pauseLoop() {
	ticker := time.NewTicker(hlsPauseCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCleanup:
			return
		case <-ticker.C:
			now := time.Now()
			m.mu.Lock()
			sessions := make([]*HLSSession, 0, len(m.sessions))
			for _, s := range m.sessions {
				sessions = append(sessions, s)
			}
			m.mu.Unlock()
			for _, s := range sessions {
				if s.pauseIfIdle(hlsPauseIdleThreshold, now) {
					slog.Debug("hls: ffmpeg suspended — client idle",
						slog.String("media_id", string(s.MediaItemID)),
						slog.String("user_id", string(s.UserID)))
				}
			}
		}
	}
}

// sweepOrphanCacheDirs removes every subdirectory of cacheDir at startup.
// No live sessions exist yet, so anything on disk is an orphan from a
// previous run (crash, kill -9, OOM). Without this sweep, orphan dirs
// accumulate forever and the 100GB+ transcode cache that shows up in prod
// is mostly dead HLS segments nobody will ever stream again.
func sweepOrphanCacheDirs(cacheDir string) {
	if cacheDir == "" {
		return
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("hls: sweep orphan dirs: read cacheDir", slog.String("dir", cacheDir), slog.Any("err", err))
		}
		return
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(cacheDir, e.Name())
		if rmErr := os.RemoveAll(path); rmErr != nil {
			slog.Warn("hls: sweep orphan dir failed", slog.String("dir", path), slog.Any("err", rmErr))
			continue
		}
		removed++
	}
	if removed > 0 {
		slog.Info("hls: orphan cache dirs swept at startup", slog.Int("count", removed), slog.String("base", cacheDir))
	}
}

func (m *HLSManager) SetMaxConcurrent(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxConcurrent = n
	// sem is NOT replaced — replacing would orphan goroutines blocked on the old channel.
	// The new limit takes effect on next restart (setting is persisted to DB).
	slog.Info("hls: max concurrent updated (effective after restart)", slog.Int("max", n))
}

func (m *HLSManager) acquireSemaphore() error {
	if m.sem == nil {
		return nil
	}
	select {
	case m.sem <- struct{}{}:
		metrics.HLSSessionsGauge.Set(float64(len(m.sem)))
		return nil
	default:
	}
	slog.Info("hls: session queued, waiting for slot", slog.Int("max", m.maxConcurrent))
	metrics.TranscodeQueueDepth.Inc()
	defer metrics.TranscodeQueueDepth.Dec()
	timer := time.NewTimer(m.queueTimeout)
	defer timer.Stop()
	select {
	case m.sem <- struct{}{}:
		metrics.HLSSessionsGauge.Set(float64(len(m.sem)))
		return nil
	case <-timer.C:
		return errors.New("too many concurrent sessions, try again")
	case <-m.shutdownCtx.Done():
		return m.shutdownCtx.Err()
	}
}

func (m *HLSManager) releaseSemaphore() {
	if m.sem == nil {
		return
	}
	select {
	case <-m.sem:
		metrics.HLSSessionsGauge.Set(float64(len(m.sem)))
	default:
	}
}

func FilterProfiles(profiles []transcoding.HLSProfile, sourceWidth, sourceHeight int) []transcoding.HLSProfile {
	var filtered []transcoding.HLSProfile
	for _, p := range profiles {
		if p.Height <= sourceHeight {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 && len(profiles) > 0 {
		filtered = []transcoding.HLSProfile{profiles[0]}
	}
	return filtered
}

func (m *HLSManager) countByUser(userID string) int {
	return m.userSessions[userID]
}

func (m *HLSManager) GetOrCreate(userID, mediaItemID, filePath string, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex int, subtitleCodec string, streams []*library.MediaStream, hdrSupported, audioNormalize bool, startSec float64, videoDecision, audioDecision string, opts ...HLSOption) (*HLSSession, error) {
	_, span := tracing.StartSpan(m.shutdownCtx, "app.transcoding.HLSManager.GetOrCreate")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("media_item.id", mediaItemID),
		attribute.String("video.decision", videoDecision),
		attribute.String("audio.decision", audioDecision),
	)
	outputDir := filepath.Join(m.cacheDir, mediaItemID)

	// Phase 1: check for matching session, or detach stale one. Lock is held
	// only long enough to mutate the in-memory maps — stop()/RemoveAll run
	// outside so a new-session start doesn't block every other caller.
	m.mu.Lock()
	if session, ok := m.sessions[mediaItemID]; ok {
		if session.VideoDecision == videoDecision && session.AudioDecision == audioDecision {
			session.MarkAccess("")
			m.mu.Unlock()
			return session, nil
		}
		old := session
		m.detachSessionLocked(old)
		m.mu.Unlock()
		slog.Info("hls: session replaced — decisions changed",
			slog.String("media_id", mediaItemID),
			slog.String("user_id", userID),
			slog.String("old_video", old.VideoDecision),
			slog.String("new_video", videoDecision),
			slog.String("old_audio", old.AudioDecision),
			slog.String("new_audio", audioDecision))
		old.stop()
		m.removeSessionDir(string(old.MediaItemID))
	} else {
		m.mu.Unlock()
	}

	if err := m.acquireSemaphore(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}

	// Phase 2: disk cache invalidation — slow, so do it unlocked. Safe
	// because no session owns outputDir yet at this point.
	if !matchesCachedDecision(outputDir, videoDecision, audioDecision) {
		if err := os.RemoveAll(outputDir); err != nil {
			slog.Warn("remove stale transcode cache failed", slog.String("dir", outputDir), slog.Any("err", err))
		}
	}

	// Phase 3: re-check + reserve the map slot (quick, under lock). Covers
	// the race where another goroutine created a session for the same
	// mediaItemID between Phase 1's unlock and now.
	m.mu.Lock()
	if session, ok := m.sessions[mediaItemID]; ok {
		if session.VideoDecision == videoDecision && session.AudioDecision == audioDecision {
			session.MarkAccess("")
			m.mu.Unlock()
			m.releaseSemaphore()
			return session, nil
		}
		old := session
		m.detachSessionLocked(old)
		m.mu.Unlock()
		slog.Info("hls: session replaced — decisions changed (race)",
			slog.String("media_id", mediaItemID),
			slog.String("user_id", userID),
			slog.String("old_video", old.VideoDecision),
			slog.String("new_video", videoDecision),
			slog.String("old_audio", old.AudioDecision),
			slog.String("new_audio", audioDecision))
		old.stop()
		m.removeSessionDir(string(old.MediaItemID))
		m.mu.Lock()
	}

	if maxUserStreams > 0 && m.countByUser(userID) >= maxUserStreams {
		m.mu.Unlock()
		m.releaseSemaphore()
		err := fmt.Errorf("max streams per user reached (%d)", maxUserStreams)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}

	profiles := FilterProfiles(transcoding.DefaultHLSProfiles, sourceWidth, sourceHeight)
	now := time.Now()
	session := &HLSSession{
		ID:                 mediaItemID,
		MediaItemID:        shared.MediaItemID(mediaItemID),
		UserID:             shared.UserID(userID),
		OutputDir:          outputDir,
		Profiles:           profiles,
		VideoDecision:      videoDecision,
		AudioDecision:      audioDecision,
		AudioTrackIndex:    audioTrackIndex,
		SubtitleTrackIndex: subtitleTrackIndex,
		SubtitleCodec:      subtitleCodec,
		HDRSupported:       hdrSupported,
		AudioNormalize:     audioNormalize,
		StartSec:           startSec,
		streams:            streams,
		StartedAt:          now,
		LastAccess:         now,
		ready:              make(chan struct{}),
		done:               make(chan struct{}),
	}
	for _, apply := range opts {
		apply(session)
	}
	m.sessions[mediaItemID] = session
	m.userSessions[userID]++
	m.mu.Unlock()

	// Phase 4: MkdirAll + start transcode — slow, unlocked. On failure,
	// roll back the map reservation so the slot doesn't leak.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		m.mu.Lock()
		m.detachSessionLocked(session)
		m.mu.Unlock()
		m.releaseSemaphore()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	if isTranscodeComplete(outputDir, session.streamCount()) {
		slog.Info("hls: reusing cached transcode", slog.String("media_id", mediaItemID))
		close(session.ready)
		close(session.done)
	} else {
		writeCachedDecision(outputDir, videoDecision, audioDecision)
		go m.startSession(session, filePath)
	}
	slog.Info("hls: session created",
		slog.String("media_id", mediaItemID),
		slog.String("user_id", userID),
		slog.String("video_decision", videoDecision),
		slog.String("audio_decision", audioDecision),
		slog.Int("audio_track", audioTrackIndex),
		slog.Int("subtitle_track", subtitleTrackIndex),
		slog.Float64("start_sec", startSec),
		slog.Int("profiles", len(profiles)),
		slog.Bool("hdr", hdrSupported))
	return session, nil
}

// detachSessionLocked removes the session from in-memory maps only.
// Stop() and RemoveAll are the caller's responsibility, performed
// outside the lock so slow I/O doesn't serialise all new-session starts.
func (m *HLSManager) detachSessionLocked(session *HLSSession) {
	delete(m.sessions, string(session.MediaItemID))
	m.userSessions[string(session.UserID)]--
	if m.userSessions[string(session.UserID)] <= 0 {
		delete(m.userSessions, string(session.UserID))
	}
}

// removeSessionDir deletes the on-disk cache for a session. Safe to call
// without the manager lock — there's no session in the map owning this
// dir by the time we get here (detachSessionLocked ran first).
func (m *HLSManager) removeSessionDir(mediaItemID string) {
	dir := filepath.Join(m.cacheDir, mediaItemID)
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("evict session cache failed", slog.String("dir", dir), slog.Any("err", err))
	}
}

func (m *HLSManager) startSession(session *HLSSession, filePath string) {
	defer close(session.done)

	opts := transcoding.HLSProcessOpts{
		OutputDir:          session.OutputDir,
		FilePath:           filePath,
		Profiles:           session.Profiles,
		VideoDecision:      session.VideoDecision,
		AudioDecision:      session.AudioDecision,
		AudioTrackIndex:    session.AudioTrackIndex,
		SubtitleTrackIndex: session.SubtitleTrackIndex,
		SubtitleCodec:      session.SubtitleCodec,
		HDRSupported:       session.HDRSupported,
		AudioNormalize:     session.AudioNormalize,
		StartSec:           session.StartSec,
		Streams:            session.streams,
		VideoCodec:         session.VideoCodec,
		AudioCodec:         session.AudioCodec,
		MaxAudioChannels:   session.MaxAudioChannels,
		ForceSidecarSubs:   session.ForceSidecarSubs,
	}

	handle, err := m.process.Start(m.shutdownCtx, opts)
	if err != nil {
		session.err = err
		close(session.ready)
		return
	}

	session.mu.Lock()
	session.handle = handle
	session.mu.Unlock()

	if err := handle.WaitReady(m.shutdownCtx); err != nil {
		session.err = err
		close(session.ready)
		return
	}

	close(session.ready)

	<-handle.Done()
}

func (m *HLSManager) GetSession(mediaItemID string) (*HLSSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[mediaItemID]
	return s, ok
}

// CacheDir returns the base directory where HLS segments are stored.
func (m *HLSManager) CacheDir() string { return m.cacheDir }

func (m *HLSManager) RestartFromPosition(userID, mediaItemID, filePath string, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex int, subtitleCodec string, streams []*library.MediaStream, hdrSupported, audioNormalize bool, startSec float64, videoDecision, audioDecision string, opts ...HLSOption) (*HLSSession, error) {
	_, span := tracing.StartSpan(m.shutdownCtx, "app.transcoding.HLSManager.RestartFromPosition")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("media_item.id", mediaItemID),
		attribute.Float64("start_sec", startSec),
	)
	m.mu.Lock()
	if old, ok := m.sessions[mediaItemID]; ok {
		delete(m.sessions, mediaItemID)
		m.userSessions[string(old.UserID)]--
		if m.userSessions[string(old.UserID)] <= 0 {
			delete(m.userSessions, string(old.UserID))
		}
		m.mu.Unlock()
		slog.Info("hls: restarting session from new position",
			slog.String("media_id", mediaItemID),
			slog.String("user_id", userID),
			slog.Float64("start_sec", startSec),
			slog.Float64("old_start_sec", old.StartSec))
		old.stop()
		m.releaseSemaphore()
	} else {
		m.mu.Unlock()
	}
	return m.GetOrCreate(userID, mediaItemID, filePath, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex, subtitleCodec, streams, hdrSupported, audioNormalize, startSec, videoDecision, audioDecision, opts...)
}

func (m *HLSManager) RemoveSession(mediaItemID string) {
	_, span := tracing.StartSpan(m.shutdownCtx, "app.transcoding.HLSManager.RemoveSession")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	m.mu.Lock()
	session, ok := m.sessions[mediaItemID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, mediaItemID)
	m.userSessions[string(session.UserID)]--
	if m.userSessions[string(session.UserID)] <= 0 {
		delete(m.userSessions, string(session.UserID))
	}
	m.mu.Unlock()
	session.stop()
	m.releaseSemaphore()
	slog.Info("hls: session removed", slog.String("media_id", mediaItemID))
}

func (m *HLSManager) ActiveSessionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *HLSManager) CleanupAll() {
	_, span := tracing.StartSpan(m.shutdownCtx, "app.transcoding.HLSManager.CleanupAll")
	defer span.End()
	m.cleanupOnce.Do(func() { close(m.stopCleanup) })
	m.mu.Lock()
	var toStop []*HLSSession
	for id, session := range m.sessions {
		toStop = append(toStop, session)
		delete(m.sessions, id)
		m.userSessions[string(session.UserID)]--
		if m.userSessions[string(session.UserID)] <= 0 {
			delete(m.userSessions, string(session.UserID))
		}
	}
	m.mu.Unlock()
	for _, s := range toStop {
		s.stop()
		m.releaseSemaphore()
	}
}

func (m *HLSManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCleanup:
			return
		case <-ticker.C:
			// Bumped from 10m to 30m: Chromecast receivers can sit on a deep
			// receiver-side buffer (pause, ad break, slow segment fetch) for
			// several minutes without hitting the server, then a wakeup fetch
			// would 404 because the session — and the output dir — were gone.
			// The cast token already lives 12h (CastStreamTokenTTL); the LRU
			// disk evictor and CacheMaxGB cap bound the disk-usage cost.
			m.cleanupStale(30 * time.Minute)
		}
	}
}

func (m *HLSManager) cleanupStale(maxIdle time.Duration) {
	// Warn-only threshold: when a session crosses 70% of maxIdle without a
	// hit, log once so we can correlate the upcoming kill with whatever the
	// receiver was doing (cast paused, network glitch, transcode stalled).
	// The flag resets on the next MarkAccess so a recovering session can
	// warn again later in its life.
	warnIdle := time.Duration(float64(maxIdle) * 0.7)
	m.mu.Lock()
	var toStop []*HLSSession
	type killed struct {
		id         string
		userID     string
		userAgent  string
		idleSecs   float64
		uptimeSecs float64
	}
	type warned struct {
		id         string
		userID     string
		userAgent  string
		idleSecs   float64
		uptimeSecs float64
	}
	var killedSessions []killed
	var warnedSessions []warned
	now := time.Now()
	for id, session := range m.sessions {
		session.mu.Lock()
		idle := now.Sub(session.LastAccess)
		uptime := now.Sub(session.StartedAt)
		ua := session.LastUserAgent
		alreadyWarned := session.warnedIdle
		if idle > warnIdle && !alreadyWarned && idle <= maxIdle {
			session.warnedIdle = true
		}
		session.mu.Unlock()
		if idle > maxIdle {
			killedSessions = append(killedSessions, killed{
				id: id, userID: string(session.UserID), userAgent: ua,
				idleSecs: idle.Seconds(), uptimeSecs: uptime.Seconds(),
			})
			toStop = append(toStop, session)
			delete(m.sessions, id)
			m.userSessions[string(session.UserID)]--
			if m.userSessions[string(session.UserID)] <= 0 {
				delete(m.userSessions, string(session.UserID))
			}
		} else if idle > warnIdle && !alreadyWarned {
			warnedSessions = append(warnedSessions, warned{
				id: id, userID: string(session.UserID), userAgent: ua,
				idleSecs: idle.Seconds(), uptimeSecs: uptime.Seconds(),
			})
		}
	}
	m.mu.Unlock()
	for _, w := range warnedSessions {
		slog.Warn("hls: session idle approaching kill threshold",
			slog.String("media_id", w.id),
			slog.String("user_id", w.userID),
			slog.String("last_user_agent", w.userAgent),
			slog.Float64("idle_seconds", w.idleSecs),
			slog.Float64("warn_threshold_seconds", warnIdle.Seconds()),
			slog.Float64("max_idle_seconds", maxIdle.Seconds()),
			slog.Float64("uptime_seconds", w.uptimeSecs))
	}
	for _, k := range killedSessions {
		slog.Info("hls: cleaning stale session",
			slog.String("media_id", k.id),
			slog.String("user_id", k.userID),
			slog.String("last_user_agent", k.userAgent),
			slog.Float64("idle_seconds", k.idleSecs),
			slog.Float64("max_idle_seconds", maxIdle.Seconds()),
			slog.Float64("uptime_seconds", k.uptimeSecs))
	}
	for _, s := range toStop {
		s.stop()
		m.releaseSemaphore()
	}
}

func isTranscodeComplete(outputDir string, numStreams int) bool {
	masterPath := filepath.Join(outputDir, "master.m3u8")
	if _, err := os.Stat(masterPath); err != nil {
		return false
	}
	for i := range numStreams {
		playlistPath := filepath.Join(outputDir, fmt.Sprintf("stream_%d.m3u8", i))
		data, err := os.ReadFile(playlistPath)
		if err != nil {
			return false
		}
		if !strings.Contains(string(data), "#EXT-X-ENDLIST") {
			return false
		}
	}
	return true
}

const decisionFileName = ".decision"

func writeCachedDecision(outputDir, videoDecision, audioDecision string) {
	path := filepath.Join(outputDir, decisionFileName)
	if err := os.WriteFile(path, []byte(videoDecision+"\n"+audioDecision), 0644); err != nil {
		slog.Warn("write cached decision failed", slog.String("path", path), slog.Any("err", err))
	}
}

func matchesCachedDecision(outputDir, videoDecision, audioDecision string) bool {
	data, err := os.ReadFile(filepath.Join(outputDir, decisionFileName))
	if err != nil {
		return false
	}
	return string(data) == videoDecision+"\n"+audioDecision
}
