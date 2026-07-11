package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// hls_audio_only.go groups the audio-only HLS process: arg building,
// StartAudioOnly, and the audioHandle lifecycle.

// BuildFFmpegAudioOnlyArgs builds the ffmpeg argument list for an audio-only
// HLS rendition. Output files are named audio_T_NNN.m4s (segments) and
// audio_T_init.mp4 (fMP4 init segment), where T is opts.SourceTrackIdx.
// The playlist is written to <OutputDir>/audio_T.m3u8.
//
// Callers MUST ensure StartNumber == floor(StartSec/3) for rafale starts so
// segment filenames match the absolute indices the VOD manifest addresses.
func BuildFFmpegAudioOnlyArgs(opts transcoding.AudioProcessOpts) []string {
	t := opts.SourceTrackIdx
	prefix := fmt.Sprintf("audio_%d", t)

	var args []string
	args = append(args, "-nostdin")

	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = appendInputModifiers(args, 0) // audio rendition is light; no input pacing
	args = append(args, "-i", opts.FilePath)

	// Suppress all video streams; only the selected audio track is muxed.
	args = append(args, "-vn")
	args = append(args, "-map", fmt.Sprintf("0:a:%d", t))

	encoder := resolveAudioEncoder(opts.AudioCodec)
	args = append(args, "-c:a", encoder)

	if opts.AudioBitrate > 0 {
		args = append(args, "-b:a", fmt.Sprintf("%dk", opts.AudioBitrate))
	}

	args = append(args, "-ac", resolveAudioChannels(opts.MaxAudioChannels))
	// 48 kHz is the HLS/CMAF standard sample rate; avoids resampler surprises
	// when the source carries 44.1 kHz or 96 kHz PCM tracks.
	args = append(args, "-ar", "48000")

	if opts.AudioNormalize {
		args = append(args, "-af", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
	}

	// HLS muxer - event-mode, fMP4 container, temp_file for atomic renames.
	args = append(args,
		"-f", "hls",
		"-hls_time", hlsSegmentSeconds,
		"-hls_list_size", "0",
		"-hls_playlist_type", "event",
		"-hls_flags", "temp_file",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", fmt.Sprintf("%s_init.mp4", prefix),
		"-hls_segment_filename", filepath.Join(opts.OutputDir, fmt.Sprintf("%s_%%03d.m4s", prefix)),
		"-start_number", fmt.Sprintf("%d", opts.StartNumber),
		filepath.Join(opts.OutputDir, fmt.Sprintf("%s.m3u8", prefix)),
	)
	return args
}

// StartAudioOnly spawns an ffmpeg audio-only HLS rendition and returns a
// handle that satisfies HLSProcessHandle. Ready is signalled once the
// audio_T.m3u8 playlist file appears on disk (same heuristic as the video
// pipeline for stream_0.m3u8). Cleanup is the caller's responsibility (same
// contract as HLSTranscoder.Start: the caller removes OutputDir at teardown).
func (t *HLSTranscoder) StartAudioOnly(ctx context.Context, opts transcoding.AudioProcessOpts) (transcoding.HLSProcessHandle, error) {
	args := BuildFFmpegAudioOnlyArgs(opts)
	slog.Info("hls: ffmpeg audio-only start",
		slog.String("media_path", opts.FilePath),
		slog.Int("track_idx", opts.SourceTrackIdx),
		slog.String("codec", opts.AudioCodec),
		slog.String("args", strings.Join(args, " ")))

	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, "ffmpeg", args...)
	stderr := newRingBuffer(8 * 1024)
	cmd.Stderr = stderr

	playlistName := fmt.Sprintf("audio_%d.m3u8", opts.SourceTrackIdx)
	h := &audioHandle{
		outputDir:    opts.OutputDir,
		playlistName: playlistName,
		cmd:          cmd,
		cancel:       cancel,
		ready:        make(chan struct{}),
		done:         make(chan struct{}),
		procExited:   make(chan struct{}),
		stderr:       stderr,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("ffmpeg audio-only start: %w", err)
	}
	// Single owner of cmd.Wait, same contract as hlsHandle.
	go func() {
		h.procWaitErr = cmd.Wait()
		close(h.procExited)
	}()

	go h.run(processCtx)
	return h, nil
}

// audioHandle controls a running audio-only HLS ffmpeg process. It satisfies
// transcoding.HLSProcessHandle so the SegmentOrchestrator can use it
// polymorphically alongside the video hlsHandle.
type audioHandle struct {
	outputDir    string
	playlistName string
	readyOnce    sync.Once // guards ready: run()'s error paths AND its panic recovery can both race to close it
	mu           sync.Mutex
	cmd          *exec.Cmd
	procExited   chan struct{}
	procWaitErr  error
	cancel       context.CancelFunc
	ready        chan struct{}
	done         chan struct{}
	err          error
	exitErr      error // raw ffmpeg exit error, for classification (guarded by mu)
	stderr       *ringBuffer
	paused       bool
}

// closeReady closes the ready channel exactly once, mirroring hlsHandle: the
// normal paths and the panic recovery below both reach it.
func (h *audioHandle) closeReady() {
	h.readyOnce.Do(func() { close(h.ready) })
}

func (h *audioHandle) setExitErr(err error) {
	h.mu.Lock()
	h.exitErr = err
	h.mu.Unlock()
}

func (h *audioHandle) awaitProcExit() error {
	<-h.procExited
	return h.procWaitErr
}

func (h *audioHandle) run(ctx context.Context) {
	defer close(h.done)
	// Same contract as hlsHandle.run: an unrecovered panic here would crash the
	// server AND strand every WaitReady()/cache.Wait() caller until its budget
	// expires. Recover, mark the session failed, unblock the waiters.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in hls audio ffmpeg run loop",
				slog.Any("recover", r), slog.String("stack", string(debug.Stack())))
			h.err = fmt.Errorf("panic in hls audio transcoder run loop: %v", r)
			h.setExitErr(h.err)
			h.cancel()
			h.closeReady()
		}
	}()

	playlistPath := filepath.Join(h.outputDir, h.playlistName)
	if err := waitForFile(ctx, h.procExited, playlistPath, 30*time.Second, 100*time.Millisecond); err != nil {
		h.err = fmt.Errorf("waiting for %s: %w", h.playlistName, err)
		h.cancel()
		we := h.awaitProcExit()
		h.setExitErr(we)
		if we != nil {
			slog.Debug("ffmpeg audio wait after cancel", slog.Any("err", we))
		}
		h.closeReady()
		return
	}

	h.closeReady()

	we := h.awaitProcExit()
	h.setExitErr(we)
	if we != nil && ctx.Err() == nil {
		slog.Warn("hls: ffmpeg audio-only exited with error",
			slog.Any("err", we),
			slog.String("stderr_tail", h.stderr.String()))
	}
}

func (h *audioHandle) WaitReady(ctx context.Context) error {
	select {
	case <-h.ready:
		return h.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *audioHandle) Stop() error {
	h.mu.Lock()
	if h.paused && h.cmd != nil && h.cmd.Process != nil {
		if err := h.cmd.Process.Signal(syscall.SIGCONT); err != nil {
			_ = err
		}
		h.paused = false
	}
	h.mu.Unlock()

	h.cancel()
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		h.mu.Lock()
		if h.cmd != nil && h.cmd.Process != nil {
			if err := h.cmd.Process.Kill(); err != nil {
				slog.Warn("ffmpeg audio kill after timeout failed", slog.String("output_dir", h.outputDir), slog.String("playlist", h.playlistName), slog.Any("err", err))
			}
		}
		h.mu.Unlock()
	}
	return nil
}

func (h *audioHandle) Done() <-chan struct{} { return h.done }
func (h *audioHandle) Err() error            { return h.err }

// Failure classifies the audio process exit. Audio-only renditions never use a
// video HW encoder, so a fault here is never a video-HW capability problem; the
// classifier still distinguishes cancellation/transient/unknown for logging.
func (h *audioHandle) Failure() transcoding.FailureClass {
	h.mu.Lock()
	e := h.exitErr
	h.mu.Unlock()
	return ClassifyHWFailure(e, h.stderr.String())
}

func (h *audioHandle) Pause() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.paused || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	if err := h.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		return err
	}
	h.paused = true
	return nil
}

func (h *audioHandle) Resume() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.paused || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	if err := h.cmd.Process.Signal(syscall.SIGCONT); err != nil {
		return err
	}
	h.paused = false
	return nil
}

// compile-time assertion: HLSTranscoder satisfies AudioProcess.
var _ transcoding.AudioProcess = (*HLSTranscoder)(nil)
