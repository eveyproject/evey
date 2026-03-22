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

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/pkg/hdr"
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
	VideoDecision       string // "copy" or "transcode"
	AudioDecision       string // "copy" or "transcode"
	AudioTrackIndex     int    // ffmpeg audio stream index (0:a:N)
	SubtitleTrackIndex  int    // -1 = no subtitle
	SubtitleCodec       string // codec name of the selected subtitle stream
	HDRSupported        bool
	AudioNormalize      bool // apply EBU R128 loudnorm filter
	StartSec            float64 // seek offset: ffmpeg -ss before -i
	streams             []*library.MediaStream
	StartedAt           time.Time
	LastAccess          time.Time
	mu                  sync.Mutex
	cmd                 *exec.Cmd
	cancel              context.CancelFunc
	ready               chan struct{}
	done                chan struct{} // closed when start() goroutine exits (process reaped)
	err                 error
}

// IsSingleStream returns true when the session produces a single stream variant
// (Mode A: video copy, or Mode B: single-quality transcode) instead of multi-variant ABR.
func (s *HLSSession) IsSingleStream() bool {
	return s.VideoDecision == "copy" || s.VideoDecision == "transcode"
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

func (m *HLSManager) GetOrCreate(userID, mediaItemID, filePath string, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex int, subtitleCodec string, streams []*library.MediaStream, hdrSupported, audioNormalize bool, startSec float64, videoDecision, audioDecision string) (*HLSSession, error) {
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
		go session.start(filePath)
	}
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

	// For single-stream mode, ffmpeg doesn't produce master.m3u8 (no -master_pl_name).
	// Wait for stream_0.m3u8, then generate a minimal master playlist pointing to it.
	if s.IsSingleStream() {
		streamPath := filepath.Join(s.OutputDir, "stream_0.m3u8")
		if err := waitForFile(ctx, streamPath, 30*time.Second, 100*time.Millisecond); err != nil {
			s.err = fmt.Errorf("waiting for stream_0.m3u8: %w", err)
			cancel()
			if waitErr := cmd.Wait(); waitErr != nil {
				slog.Warn("hls: ffmpeg wait failed", slog.String("media_id", s.MediaItemID), slog.Any("err", waitErr))
			}
			close(s.ready)
			return
		}
		master := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nstream_0.m3u8\n"
		if err := os.WriteFile(filepath.Join(s.OutputDir, "master.m3u8"), []byte(master), 0644); err != nil {
			s.err = fmt.Errorf("write master.m3u8: %w", err)
			cancel()
			if waitErr := cmd.Wait(); waitErr != nil {
				slog.Warn("hls: ffmpeg wait failed", slog.String("media_id", s.MediaItemID), slog.Any("err", waitErr))
			}
			close(s.ready)
			return
		}
	} else {
		masterPath := filepath.Join(s.OutputDir, "master.m3u8")
		if err := waitForFile(ctx, masterPath, 30*time.Second, 100*time.Millisecond); err != nil {
			s.err = fmt.Errorf("waiting for master.m3u8: %w", err)
			cancel()
			if waitErr := cmd.Wait(); waitErr != nil {
				slog.Warn("hls: ffmpeg wait failed", slog.String("media_id", s.MediaItemID), slog.Any("err", waitErr))
			}
			close(s.ready)
			return
		}
	}

	close(s.ready)

	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("hls: ffmpeg exited", slog.String("media_id", s.MediaItemID), slog.Any("err", err))
	}
}

func (s *HLSSession) BuildFFmpegArgs(filePath string) []string {
	return s.buildFFmpegArgs(filePath)
}

// streamCount returns the number of HLS stream variants this session produces.
func (s *HLSSession) streamCount() int {
	if s.IsSingleStream() {
		return 1
	}
	return len(s.Profiles)
}

func (s *HLSSession) buildFFmpegArgs(filePath string) []string {
	if s.VideoDecision == "copy" {
		return s.buildCopyVideoArgs(filePath)
	}
	if s.VideoDecision == "transcode" {
		return s.buildSingleTranscodeArgs(filePath)
	}
	return s.buildMultiVariantArgs(filePath)
}

// buildCopyVideoArgs — Mode A: video copy + audio transcode/copy. Single stream, no filter_complex.
// -ss BEFORE -i (input seeking) for instant seek — lands on nearest keyframe.
func (s *HLSSession) buildCopyVideoArgs(filePath string) []string {
	var args []string
	if s.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", s.StartSec))
	}
	args = append(args, "-i", filePath)
	args = append(args,
		"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d", s.AudioTrackIndex),
		"-c:v", "copy",
	)
	if s.AudioDecision == "transcode" {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
		if s.AudioNormalize {
			args = append(args, "-af", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
		}
	} else {
		args = append(args, "-c:a", "copy")
	}
	args = append(args,
		"-f", "hls",
		"-hls_time", "3",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(s.OutputDir, "stream_0_%03d.ts"),
		filepath.Join(s.OutputDir, "stream_0.m3u8"),
	)
	return args
}

// buildSingleTranscodeArgs — Mode B: single-quality video transcode at best matching profile resolution.
func (s *HLSSession) buildSingleTranscodeArgs(filePath string) []string {
	burnIn := s.SubtitleTrackIndex >= 0 && IsImageSubtitle(s.SubtitleCodec)

	toneMap := s.detectToneMap()

	var args []string
	if s.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", s.StartSec))
	}
	args = append(args, "-i", filePath)

	// Pick the highest-resolution profile that fits the source
	p := s.Profiles[len(s.Profiles)-1]

	if burnIn || toneMap != "" {
		var filterParts []string
		switch {
		case burnIn && toneMap != "":
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vt]", toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay,scale=%d:-2,setsar=1[out]", s.SubtitleTrackIndex, p.Width),
			)
		case burnIn:
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v][0:s:%d]overlay,scale=%d:-2,setsar=1[out]", s.SubtitleTrackIndex, p.Width),
			)
		default: // toneMap only
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,scale=%d:-2,setsar=1[out]", toneMap, p.Width),
			)
		}
		args = append(args, "-filter_complex", strings.Join(filterParts, ";"))
		args = append(args, "-map", "[out]")
	} else {
		args = append(args, "-map", "0:v:0",
			"-vf", fmt.Sprintf("scale=%d:-2,setsar=1", p.Width),
		)
	}

	args = append(args, "-map", fmt.Sprintf("0:a:%d", s.AudioTrackIndex),
		"-c:v", "libx264",
		"-profile:v", "high", "-level:v", "4.1",
		"-b:v", fmt.Sprintf("%dk", p.VideoBitrate),
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
	)

	if s.AudioDecision == "transcode" {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", fmt.Sprintf("%dk", p.AudioBitrate))
		if s.AudioNormalize {
			args = append(args, "-filter:a:0", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
		}
	} else {
		args = append(args, "-c:a", "copy")
	}

	args = append(args,
		"-f", "hls",
		"-hls_time", "3",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(s.OutputDir, "stream_0_%03d.ts"),
		filepath.Join(s.OutputDir, "stream_0.m3u8"),
	)
	return args
}

// buildMultiVariantArgs — legacy multi-variant ABR (4 quality levels).
func (s *HLSSession) buildMultiVariantArgs(filePath string) []string {
	n := len(s.Profiles)
	burnIn := s.SubtitleTrackIndex >= 0 && IsImageSubtitle(s.SubtitleCodec)

	toneMap := s.detectToneMap()

	var args []string
	if s.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", s.StartSec))
	}
	args = append(args, "-i", filePath)

	var filterParts []string
	splitOutputs := make([]string, n)
	for i := range n {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}

	if burnIn {
		if toneMap != "" {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vt]", toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay[vb]", s.SubtitleTrackIndex),
				fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		} else {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v][0:s:%d]overlay[vb]", s.SubtitleTrackIndex),
				fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		}
	} else {
		if toneMap != "" {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vt]", toneMap),
				fmt.Sprintf("[vt]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		} else {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		}
	}

	for i, p := range s.Profiles {
		filterParts = append(filterParts, fmt.Sprintf("[v%d]scale=%d:-2,setsar=1[o%d]", i, p.Width, i))
	}
	args = append(args, "-filter_complex", strings.Join(filterParts, ";"))

	for i, p := range s.Profiles {
		args = append(args,
			"-map", fmt.Sprintf("[o%d]", i),
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-b:v:%d", i), fmt.Sprintf("%dk", p.VideoBitrate),
			"-map", fmt.Sprintf("0:a:%d", s.AudioTrackIndex),
			fmt.Sprintf("-c:a:%d", i), "aac", "-ac", "2",
			fmt.Sprintf("-b:a:%d", i), fmt.Sprintf("%dk", p.AudioBitrate),
		)
		if s.AudioNormalize {
			args = append(args, fmt.Sprintf("-filter:a:%d", i), "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
		}
	}

	var streamMap []string
	for i := range n {
		streamMap = append(streamMap, fmt.Sprintf("v:%d,a:%d", i, i))
	}
	args = append(args, "-var_stream_map", strings.Join(streamMap, " "))

	args = append(args,
		"-master_pl_name", "master.m3u8",
		"-f", "hls",
		"-hls_time", "3",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(s.OutputDir, "stream_%v_%03d.ts"),
		filepath.Join(s.OutputDir, "stream_%v.m3u8"),
	)
	return args
}

func (s *HLSSession) detectToneMap() string {
	if s.HDRSupported {
		return ""
	}
	hdrStreams := make([]hdr.VideoStream, len(s.streams))
	for i, st := range s.streams {
		hdrStreams[i] = st
	}
	hdrInfo := hdr.DetectHDR(hdrStreams)
	if hdrInfo.IsHDR {
		return hdr.BuildToneMapFilter(hdrInfo, hdr.DefaultToneMapOptions())
	}
	return ""
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
			if err := s.cmd.Process.Kill(); err != nil {
				slog.Warn("hls: kill ffmpeg failed", slog.String("media_id", s.MediaItemID), slog.Any("err", err))
			}
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
	} else {
		m.mu.Unlock()
	}
	return m.GetOrCreate(userID, mediaItemID, filePath, sourceWidth, sourceHeight, maxUserStreams, audioTrackIndex, subtitleTrackIndex, subtitleCodec, streams, hdrSupported, audioNormalize, startSec, videoDecision, audioDecision)
}

func (m *HLSManager) ActiveSessionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// isTranscodeComplete checks if a previous transcode exists and is fully finished
// (all stream playlists have #EXT-X-ENDLIST). If so, we can serve the cached files
// instead of re-encoding.
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
