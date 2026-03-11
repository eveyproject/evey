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
	ID          string
	MediaItemID string
	OutputDir   string
	Profiles    []HLSProfile
	StartedAt   time.Time
	LastAccess  time.Time
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	ready       chan struct{}
	err         error
}

func (s *HLSSession) MarkAccess() {
	s.mu.Lock()
	s.LastAccess = time.Now()
	s.mu.Unlock()
}

type HLSManager struct {
	mu            sync.Mutex
	sessions      map[string]*HLSSession
	cacheDir      string
	maxConcurrent int
	stopCleanup   chan struct{}
	cleanupOnce   sync.Once
}

func NewHLSManager(cacheDir string, maxConcurrent int) *HLSManager {
	m := &HLSManager{
		sessions:      make(map[string]*HLSSession),
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

func (m *HLSManager) GetOrCreate(mediaItemID, filePath string, sourceWidth, sourceHeight int) (*HLSSession, error) {
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

	profiles := FilterProfiles(DefaultProfiles, sourceWidth, sourceHeight)
	outputDir := filepath.Join(m.cacheDir, mediaItemID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	now := time.Now()
	session := &HLSSession{
		ID:          mediaItemID,
		MediaItemID: mediaItemID,
		OutputDir:   outputDir,
		Profiles:    profiles,
		StartedAt:   now,
		LastAccess:  now,
		ready:       make(chan struct{}),
	}
	m.sessions[mediaItemID] = session

	go session.start(filePath)
	return session, nil
}

func (s *HLSSession) start(filePath string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	args := s.buildFFmpegArgs(filePath)
	slog.Debug("hls: ffmpeg", slog.String("media_id", s.MediaItemID), slog.String("args", strings.Join(args, " ")))

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = nil
	s.mu.Lock()
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
		cmd.Wait()
		close(s.ready)
		return
	}

	close(s.ready)

	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("hls: ffmpeg exited", slog.String("media_id", s.MediaItemID), slog.Any("err", err))
	}
}

func (s *HLSSession) buildFFmpegArgs(filePath string) []string {
	n := len(s.Profiles)
	args := []string{"-i", filePath}

	// Build filter_complex: split video and scale for each profile
	var filterParts []string
	splitOutputs := make([]string, n)
	for i := range n {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}
	filterParts = append(filterParts, fmt.Sprintf("[0:v]split=%d%s", n, strings.Join(splitOutputs, "")))
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
	os.RemoveAll(s.OutputDir)
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
	defer m.mu.Unlock()

	now := time.Now()
	for id, session := range m.sessions {
		session.mu.Lock()
		idle := now.Sub(session.LastAccess)
		session.mu.Unlock()
		if idle > maxIdle {
			slog.Info("hls: cleaning stale session", slog.String("media_id", id))
			session.stop()
			delete(m.sessions, id)
		}
	}
}

func (m *HLSManager) CleanupAll() {
	m.cleanupOnce.Do(func() { close(m.stopCleanup) })
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		session.stop()
		delete(m.sessions, id)
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
