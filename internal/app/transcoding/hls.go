package transcoding

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func IsImageSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub":
		return true
	}
	return false
}

type HLSProfile struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate int64 // kbps
	AudioBitrate int64 // kbps
}

var DefaultProfiles = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400, AudioBitrate: 128},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
}

type HLSSession struct {
	ID                  string
	MediaItemID         string
	UserID              string
	OutputDir           string
	Profiles            []HLSProfile
	SubtitleTrackIndex  int    // -1 = no subtitle
	SubtitleCodec       string // codec name of the selected subtitle stream
	StartedAt           time.Time
	LastAccess          time.Time
	mu                  sync.Mutex
	cmd                 *exec.Cmd
	cancel              context.CancelFunc
	ready               chan struct{}
	done                chan struct{} // closed when start() goroutine exits (process reaped)
	err                 error
}

func (s *HLSSession) MarkAccess() {
	s.mu.Lock()
	s.LastAccess = time.Now()
	s.mu.Unlock()
}

type HLSManager struct {
	mu            sync.Mutex
	sessions      map[string]*HLSSession
	userSessions  map[string]int // userID → active session count (O(1) lookup)
	cacheDir      string
	maxConcurrent int
	stopCleanup   chan struct{}
	cleanupOnce   sync.Once
}

func NewHLSManager(cacheDir string, maxConcurrent int) *HLSManager {
	m := &HLSManager{
		sessions:      make(map[string]*HLSSession),
		userSessions:  make(map[string]int),
		cacheDir:      cacheDir,
		maxConcurrent: maxConcurrent,
		stopCleanup:   make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

func FilterProfiles(profiles []HLSProfile, sourceWidth, sourceHeight int) []HLSProfile {
	var filtered []HLSProfile
	for _, p := range profiles {
		if p.Width <= sourceWidth && p.Height <= sourceHeight {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 && len(profiles) > 0 {
		filtered = []HLSProfile{profiles[0]}
	}
	return filtered
}

// countByUser returns the number of active sessions for the given user (O(1)).
// Must be called with m.mu held.
func (m *HLSManager) countByUser(userID string) int {
	return m.userSessions[userID]
}

func (m *HLSManager) GetOrCreate(userID, mediaItemID, filePath string, sourceWidth, sourceHeight, maxUserStreams, subtitleTrackIndex int, subtitleCodec string) (*HLSSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[mediaItemID]; ok {
		session.MarkAccess()
		return session, nil
	}

	active := len(m.sessions)
	if m.maxConcurrent > 0 && active >= m.maxConcurrent {
		return nil, fmt.Errorf("max concurrent HLS sessions reached (%d)", m.maxConcurrent)
	}
	if maxUserStreams > 0 && m.countByUser(userID) >= maxUserStreams {
		return nil, fmt.Errorf("max streams per user reached (%d)", maxUserStreams)
	}

	profiles := FilterProfiles(DefaultProfiles, sourceWidth, sourceHeight)
	outputDir := filepath.Join(m.cacheDir, mediaItemID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	now := time.Now()
	session := &HLSSession{
		ID:                 mediaItemID,
		MediaItemID:        mediaItemID,
		UserID:             userID,
		OutputDir:          outputDir,
		Profiles:           profiles,
		SubtitleTrackIndex: subtitleTrackIndex,
		SubtitleCodec:      subtitleCodec,
		StartedAt:          now,
		LastAccess:         now,
		ready:              make(chan struct{}),
		done:               make(chan struct{}),
	}
	m.sessions[mediaItemID] = session
	m.userSessions[userID]++

	go session.start(filePath)
	return session, nil
}

func (s *HLSSession) start(filePath string) {
	defer close(s.done)
	ctx, cancel := context.WithCancel(context.Background())
	args := s.buildFFmpegArgs(filePath)
	slog.Debug("hls: ffmpeg", slog.String("media_id", s.MediaItemID), slog.String("args", strings.Join(args, " ")))
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = nil

	s.mu.Lock()
	s.cancel = cancel
	s.cmd = cmd
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.err = fmt.Errorf("ffmpeg start: %w", err)
		close(s.ready)
		return
	}

	masterPath := filepath.Join(s.OutputDir, "master.m3u8")
	if err := waitForFile(ctx, masterPath, 30*time.Second, 500*time.Millisecond); err != nil {
		s.err = fmt.Errorf("waiting for master.m3u8: %w", err)
		cancel()
		if waitErr := cmd.Wait(); waitErr != nil {
			slog.Warn("hls: ffmpeg wait failed", slog.String("media_id", s.MediaItemID), slog.Any("err", waitErr))
		}
		close(s.ready)
		return
	}

	close(s.ready)

	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("hls: ffmpeg exited", slog.String("media_id", s.MediaItemID), slog.Any("err", err))
	}
}

func (s *HLSSession) BuildFFmpegArgs(filePath string) []string {
	return s.buildFFmpegArgs(filePath)
}

func (s *HLSSession) buildFFmpegArgs(filePath string) []string {
	n := len(s.Profiles)
	burnIn := s.SubtitleTrackIndex >= 0 && IsImageSubtitle(s.SubtitleCodec)

	args := []string{"-i", filePath}

	// Build filter_complex
	var filterParts []string
	splitOutputs := make([]string, n)
	for i := range n {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}

	if burnIn {
		// overlay subtitle stream onto video before splitting
		filterParts = append(filterParts,
			fmt.Sprintf("[0:v][0:s:%d]overlay[vb]", s.SubtitleTrackIndex),
			fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
		)
	} else {
		filterParts = append(filterParts,
			fmt.Sprintf("[0:v]split=%d%s", n, strings.Join(splitOutputs, "")),
		)
	}

	for i, p := range s.Profiles {
		filterParts = append(filterParts, fmt.Sprintf("[v%d]scale=%d:%d[o%d]", i, p.Width, p.Height, i))
	}
	args = append(args, "-filter_complex", strings.Join(filterParts, ";"))

	// Map each scaled video + first audio, set codecs/bitrates
	for i, p := range s.Profiles {
		args = append(args,
			"-map", fmt.Sprintf("[o%d]", i),
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-b:v:%d", i), fmt.Sprintf("%dk", p.VideoBitrate),
			"-map", "0:a:0",
			fmt.Sprintf("-c:a:%d", i), "aac",
			fmt.Sprintf("-b:a:%d", i), fmt.Sprintf("%dk", p.AudioBitrate),
		)
	}

	// var_stream_map
	var streamMap []string
	for i := range n {
		streamMap = append(streamMap, fmt.Sprintf("v:%d,a:%d", i, i))
	}
	args = append(args, "-var_stream_map", strings.Join(streamMap, " "))

	args = append(args,
		"-master_pl_name", "master.m3u8",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(s.OutputDir, "stream_%v_%03d.ts"),
		filepath.Join(s.OutputDir, "stream_%v.m3u8"),
	)
	return args
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
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// Wait for start() goroutine to call cmd.Wait() and reap the ffmpeg process.
	// Use a timeout so a stuck process doesn't block shutdown indefinitely.
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		slog.Warn("hls: timed out waiting for ffmpeg to exit, killing process", slog.String("media_id", s.MediaItemID))
		s.mu.Lock()
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		s.mu.Unlock()
	}
	if err := os.RemoveAll(s.OutputDir); err != nil {
		slog.Warn("hls: remove output dir failed", slog.String("dir", s.OutputDir), slog.Any("err", err))
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
			m.cleanupStale(30 * time.Minute)
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
	}
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
	}
}

func (m *HLSManager) GetSession(mediaItemID string) (*HLSSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[mediaItemID]
	return s, ok
}

func (m *HLSManager) ActiveSessionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func waitForFile(ctx context.Context, path string, timeout, interval time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for %s", path)
		case <-ticker.C:
			if _, err := os.Stat(path); err == nil {
				return nil
			}
		}
	}
}
