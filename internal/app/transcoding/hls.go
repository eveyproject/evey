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

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

func IsImageSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub":
		return true
	}
	return false
}

type HLSSession struct {
	ID                  string
	MediaItemID         string
	UserID              string
	OutputDir           string
	Profiles            []transcoding.HLSProfile
	VideoDecision       string // "copy" or "transcode"
	AudioDecision       string // "copy" or "transcode"
	AudioTrackIndex     int    // ffmpeg audio stream index (0:a:N)
	SubtitleTrackIndex  int    // -1 = no subtitle
	SubtitleCodec       string
	HDRSupported        bool
	AudioNormalize      bool
	StartSec            float64 // seek offset
	streams             []*library.MediaStream
	StartedAt           time.Time
	LastAccess          time.Time
	mu                  sync.Mutex
	handle              transcoding.HLSProcessHandle // nil when serving cached transcode
	ready               chan struct{}
	done                chan struct{}
	err                 error
}

func (s *HLSSession) IsSingleStream() bool {
	return s.VideoDecision == "copy"
}

func (s *HLSSession) MarkAccess() {
	s.mu.Lock()
	s.LastAccess = time.Now()
	s.mu.Unlock()
}

func (s *HLSSession) streamCount() int {
	if s.IsSingleStream() {
		return 1
	}
	return len(s.Profiles)
}

func (s *HLSSession) WaitReady(ctx context.Context) error {
	select {
	case <-s.ready:
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *HLSSession) stop() {
	s.mu.Lock()
	h := s.handle
	s.mu.Unlock()
	if h != nil {
		if err := h.Stop(); err != nil {
			slog.Warn("hls: stop handle failed", slog.String("media_id", s.MediaItemID), slog.Any("err", err))
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
	go m.cleanupLoop()
	return m
}

func (m *HLSManager) SetMaxConcurrent(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxConcurrent = n
	if n > 0 {
		m.sem = make(chan struct{}, n)
	} else {
		m.sem = nil
	}
	slog.Info("hls: updated max concurrent", slog.Int("max", n))
}

func (m *HLSManager) acquireSemaphore() error {
	if m.sem == nil {
		return nil
	}
	select {
	case m.sem <- struct{}{}:
		return nil
	default:
	}
	slog.Info("hls: session queued, waiting for slot", slog.Int("max", m.maxConcurrent))
	timer := time.NewTimer(m.queueTimeout)
	defer timer.Stop()
	select {
	case m.sem <- struct{}{}:
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

func (m *HLSManager) GetOrCreate(userID, mediaItemID, filePath string, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex int, subtitleCodec string, streams []*library.MediaStream, hdrSupported, audioNormalize bool, startSec float64, videoDecision, audioDecision string) (*HLSSession, error) {
	m.mu.Lock()
	if session, ok := m.sessions[mediaItemID]; ok {
		if session.VideoDecision == videoDecision && session.AudioDecision == audioDecision {
			session.MarkAccess()
			m.mu.Unlock()
			return session, nil
		}
		// Decision mismatch (different client capabilities) — evict stale session
		m.evictSessionLocked(session)
		m.mu.Unlock()
		session.stop()
	} else {
		m.mu.Unlock()
	}

	if err := m.acquireSemaphore(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-check: session may have been created while waiting
	if session, ok := m.sessions[mediaItemID]; ok {
		if session.VideoDecision == videoDecision && session.AudioDecision == audioDecision {
			session.MarkAccess()
			m.releaseSemaphore()
			return session, nil
		}
		m.evictSessionLocked(session)
		session.stop()
	}

	if maxUserStreams > 0 && m.countByUser(userID) >= maxUserStreams {
		m.releaseSemaphore()
		return nil, fmt.Errorf("max streams per user reached (%d)", maxUserStreams)
	}

	profiles := FilterProfiles(transcoding.DefaultHLSProfiles, sourceWidth, sourceHeight)
	outputDir := filepath.Join(m.cacheDir, mediaItemID)

	// Invalidate disk cache if decisions don't match
	if !matchesCachedDecision(outputDir, videoDecision, audioDecision) {
		_ = os.RemoveAll(outputDir)
	}

	now := time.Now()
	session := &HLSSession{
		ID:                 mediaItemID,
		MediaItemID:        mediaItemID,
		UserID:             userID,
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
	m.sessions[mediaItemID] = session
	m.userSessions[userID]++

	if err := os.MkdirAll(outputDir, 0755); err != nil {
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
	return session, nil
}

func (m *HLSManager) evictSessionLocked(session *HLSSession) {
	delete(m.sessions, session.MediaItemID)
	m.userSessions[session.UserID]--
	if m.userSessions[session.UserID] <= 0 {
		delete(m.userSessions, session.UserID)
	}
	_ = os.RemoveAll(filepath.Join(m.cacheDir, session.MediaItemID))
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

func (m *HLSManager) RestartFromPosition(userID, mediaItemID, filePath string, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex int, subtitleCodec string, streams []*library.MediaStream, hdrSupported, audioNormalize bool, startSec float64, videoDecision, audioDecision string) (*HLSSession, error) {
	m.mu.Lock()
	if old, ok := m.sessions[mediaItemID]; ok {
		delete(m.sessions, mediaItemID)
		m.userSessions[old.UserID]--
		if m.userSessions[old.UserID] <= 0 {
			delete(m.userSessions, old.UserID)
		}
		m.mu.Unlock()
		old.stop()
		m.releaseSemaphore()
	} else {
		m.mu.Unlock()
	}
	return m.GetOrCreate(userID, mediaItemID, filePath, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex, subtitleCodec, streams, hdrSupported, audioNormalize, startSec, videoDecision, audioDecision)
}

func (m *HLSManager) RemoveSession(mediaItemID string) {
	m.mu.Lock()
	session, ok := m.sessions[mediaItemID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, mediaItemID)
	m.userSessions[session.UserID]--
	if m.userSessions[session.UserID] <= 0 {
		delete(m.userSessions, session.UserID)
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
	m.cleanupOnce.Do(func() { close(m.stopCleanup) })
	m.mu.Lock()
	var toStop []*HLSSession
	for id, session := range m.sessions {
		toStop = append(toStop, session)
		delete(m.sessions, id)
		m.userSessions[session.UserID]--
		if m.userSessions[session.UserID] <= 0 {
			delete(m.userSessions, session.UserID)
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
			m.cleanupStale(10 * time.Minute)
		}
	}
}

func (m *HLSManager) cleanupStale(maxIdle time.Duration) {
	m.mu.Lock()
	var toStop []*HLSSession
	now := time.Now()
	for id, session := range m.sessions {
		session.mu.Lock()
		idle := now.Sub(session.LastAccess)
		session.mu.Unlock()
		if idle > maxIdle {
			slog.Info("hls: cleaning stale session", slog.String("media_id", id))
			toStop = append(toStop, session)
			delete(m.sessions, id)
			m.userSessions[session.UserID]--
			if m.userSessions[session.UserID] <= 0 {
				delete(m.userSessions, session.UserID)
			}
		}
	}
	m.mu.Unlock()
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
	_ = os.WriteFile(filepath.Join(outputDir, decisionFileName), []byte(videoDecision+"\n"+audioDecision), 0644)
}

func matchesCachedDecision(outputDir, videoDecision, audioDecision string) bool {
	data, err := os.ReadFile(filepath.Join(outputDir, decisionFileName))
	if err != nil {
		return false
	}
	return string(data) == videoDecision+"\n"+audioDecision
}
