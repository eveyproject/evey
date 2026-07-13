package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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

	// Scale the (stereo-sized) base to the channel target with the SAME rule as
	// the muxed ladder (audioOutputArgsSingle): a 5.1 track kept at the stereo
	// base would ship 6 channels at 128 kbps. Deterministic in
	// (AudioBitrate, MaxAudioChannels) - both hashed into the AudioFingerprint -
	// so a given fingerprint still yields the same bytes.
	if opts.AudioBitrate > 0 {
		args = append(args, "-b:a",
			fmt.Sprintf("%dk", scaledAudioBitrateKbps(int64(opts.AudioBitrate), opts.MaxAudioChannels)))
	}

	args = append(args, "-ac", resolveAudioChannels(opts.MaxAudioChannels))
	// 48 kHz is the HLS/CMAF standard sample rate; avoids resampler surprises
	// when the source carries 44.1 kHz or 96 kHz PCM tracks.
	args = append(args, "-ar", "48000")

	// Land on the VIDEO's timeline. A video with B-frames is presented late by
	// has_b_frames/fps (MP4 forbids a negative DTS); muxed, ffmpeg shifted both
	// streams together, but a separate rendition starts at 0 and would run early
	// by exactly that much. -output_ts_offset shifts the OUTPUT stamps - what the
	// player reads - rather than the input, so the decoded content is unchanged
	// and only its position on the timeline moves.
	if opts.VideoCompositionDelay > 0 {
		args = append(args, "-output_ts_offset", fmt.Sprintf("%.3f", opts.VideoCompositionDelay))
	}

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

// needsAudioFMP4Rebase reports whether this audio session's fragments must have
// their timestamps shifted to absolute time. movenc writes tfdt session-relative
// ALWAYS (see fmp4_rebase.go), so a seeked session (-ss / -start_number > 0)
// emits tfdt=0 on its first fragment: the player would drop 30-minutes-in audio
// at the start of the timeline while the video sits correctly at the seek point.
// A cold session starts at 0, where session-relative already IS absolute.
func needsAudioFMP4Rebase(o transcoding.AudioProcessOpts) bool {
	return o.StartNumber > 0 || o.StartSec > 0
}

// StartAudioOnly spawns an ffmpeg audio-only HLS rendition and returns a
// handle that satisfies HLSProcessHandle. Ready is signalled once the
// audio_T.m3u8 playlist file appears on disk (same heuristic as the video
// pipeline for stream_0.m3u8). Cleanup is the caller's responsibility (same
// contract as HLSTranscoder.Start: the caller removes OutputDir at teardown).
func (t *HLSTranscoder) StartAudioOnly(ctx context.Context, opts transcoding.AudioProcessOpts) (transcoding.HLSProcessHandle, error) {
	// The composition delay is the MUXER's, not the source's:
	//   - mkvgo (>= the elst fix): presents its first frame at 0  -> NO shift needed
	//   - ffmpeg -c:v copy in fMP4: presents at has_b_frames/fps  -> shift needed
	// The hybrid rung (the only one serving renditions) has its video packaged by
	// mkvgo, so no shift. TODO: when the video falls back to the ffmpeg copy
	// producer (mkvgo not eligible), the shift IS still required - the caller must
	// then set VideoCompositionDelay explicitly. Left probe-free here rather than
	// re-introducing the ffprobe call the mkvgo fix made unnecessary.
	// Seeked sessions write into a private subdir; the rebase pump shifts each
	// fragment's tfdt/sidx by the session's absolute start and atomically renames
	// into the cache dir - the rename is the publish point SegmentCache.IsReady
	// observes, so no reader ever sees a pre-rebase (t=0) fragment.
	pubDir := opts.OutputDir
	rebase := needsAudioFMP4Rebase(opts)
	if rebase {
		priv := filepath.Join(pubDir, fmt.Sprintf(".rebase_audio_%d_%d_%d",
			opts.SourceTrackIdx, opts.StartNumber, time.Now().UnixNano()))
		if err := os.MkdirAll(priv, 0o755); err != nil {
			return nil, fmt.Errorf("audio rebase dir: %w", err)
		}
		opts.OutputDir = priv
	}

	args := BuildFFmpegAudioOnlyArgs(opts)
	slog.Info("hls: ffmpeg audio-only start",
		slog.String("media_path", opts.FilePath),
		slog.Int("track_idx", opts.SourceTrackIdx),
		slog.String("codec", opts.AudioCodec),
		slog.Int("start_seg", opts.StartNumber),
		slog.Bool("rebase", rebase),
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
	if rebase {
		// variantIdx = the audio track: the pump's segment matcher and metrics
		// are per-rendition. No expected-start tripwire on audio (the video
		// pipeline's copy plan supplies one; audio has no such plan).
		deltaMs := int64(opts.StartSec*1000 + 0.5)
		go runRebasePump(h.done, processCtx.Done(), opts.OutputDir, pubDir, deltaMs, "audio", opts.SourceTrackIdx, nil, "audio")
	}
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
