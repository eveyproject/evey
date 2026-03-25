package ffmpeg

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
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/pkg/hdr"
)

var _ transcoding.HLSProcess = (*HLSTranscoder)(nil)

type HLSTranscoder struct{}

func NewHLSTranscoder() *HLSTranscoder {
	return &HLSTranscoder{}
}

func (t *HLSTranscoder) Start(ctx context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	args := BuildFFmpegArgs(opts)
	slog.Debug("hls: ffmpeg", slog.String("args", strings.Join(args, " ")))

	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, "ffmpeg", args...)
	cmd.Stderr = nil

	h := &hlsHandle{
		opts:   opts,
		cmd:    cmd,
		cancel: cancel,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	go h.run(processCtx)
	return h, nil
}

type hlsHandle struct {
	opts   transcoding.HLSProcessOpts
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	ready  chan struct{}
	done   chan struct{}
	err    error
}

func (h *hlsHandle) run(ctx context.Context) {
	defer close(h.done)

	isSingle := h.opts.VideoDecision == "copy" || h.opts.VideoDecision == "transcode"

	if isSingle {
		streamPath := filepath.Join(h.opts.OutputDir, "stream_0.m3u8")
		if err := waitForFile(ctx, streamPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for stream_0.m3u8: %w", err)
			h.cancel()
			_ = h.cmd.Wait()
			close(h.ready)
			return
		}
		master := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nstream_0.m3u8\n"
		if err := os.WriteFile(filepath.Join(h.opts.OutputDir, "master.m3u8"), []byte(master), 0644); err != nil {
			h.err = fmt.Errorf("write master.m3u8: %w", err)
			h.cancel()
			_ = h.cmd.Wait()
			close(h.ready)
			return
		}
	} else {
		masterPath := filepath.Join(h.opts.OutputDir, "master.m3u8")
		if err := waitForFile(ctx, masterPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for master.m3u8: %w", err)
			h.cancel()
			_ = h.cmd.Wait()
			close(h.ready)
			return
		}
	}

	close(h.ready)

	if err := h.cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("hls: ffmpeg exited with error", slog.Any("err", err))
	}
}

func (h *hlsHandle) WaitReady(ctx context.Context) error {
	select {
	case <-h.ready:
		return h.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *hlsHandle) Stop() error {
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		h.mu.Lock()
		if h.cmd != nil && h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
		h.mu.Unlock()
	}
	return nil
}

func (h *hlsHandle) Done() <-chan struct{} {
	return h.done
}

func (h *hlsHandle) Err() error {
	return h.err
}

// --- ffmpeg arg building ---

func IsImageSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub":
		return true
	}
	return false
}

func BuildFFmpegArgs(opts transcoding.HLSProcessOpts) []string {
	if opts.VideoDecision == "copy" {
		return buildCopyVideoArgs(opts)
	}
	if opts.VideoDecision == "transcode" {
		return buildSingleTranscodeArgs(opts)
	}
	return buildMultiVariantArgs(opts)
}

func buildCopyVideoArgs(opts transcoding.HLSProcessOpts) []string {
	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = append(args, "-i", opts.FilePath)
	args = append(args,
		"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
		"-c:v", "copy",
	)
	if opts.AudioDecision == "transcode" {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
		if opts.AudioNormalize {
			args = append(args, "-af", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
		}
	} else {
		args = append(args, "-c:a", "copy")
	}
	args = append(args,
		"-f", "hls",
		"-hls_time", "3",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(opts.OutputDir, "stream_0_%03d.ts"),
		filepath.Join(opts.OutputDir, "stream_0.m3u8"),
	)
	return args
}

func buildSingleTranscodeArgs(opts transcoding.HLSProcessOpts) []string {
	burnIn := opts.SubtitleTrackIndex >= 0 && IsImageSubtitle(opts.SubtitleCodec)
	toneMap := detectToneMap(opts.HDRSupported, opts.Streams)

	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = append(args, "-i", opts.FilePath)

	p := opts.Profiles[len(opts.Profiles)-1]

	if burnIn || toneMap != "" {
		var filterParts []string
		switch {
		case burnIn && toneMap != "":
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vt]", toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay,scale=%d:-2,setsar=1[out]", opts.SubtitleTrackIndex, p.Width),
			)
		case burnIn:
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v][0:s:%d]overlay,scale=%d:-2,setsar=1[out]", opts.SubtitleTrackIndex, p.Width),
			)
		default:
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

	args = append(args, "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
		"-c:v", "libx264",
		"-profile:v", "high", "-level:v", "4.1",
		"-b:v", fmt.Sprintf("%dk", p.VideoBitrate),
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
	)

	if opts.AudioDecision == "transcode" {
		args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", fmt.Sprintf("%dk", p.AudioBitrate))
		if opts.AudioNormalize {
			args = append(args, "-filter:a:0", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
		}
	} else {
		args = append(args, "-c:a", "copy")
	}

	args = append(args,
		"-f", "hls",
		"-hls_time", "3",
		"-hls_list_size", "0",
		"-hls_segment_filename", filepath.Join(opts.OutputDir, "stream_0_%03d.ts"),
		filepath.Join(opts.OutputDir, "stream_0.m3u8"),
	)
	return args
}

func buildMultiVariantArgs(opts transcoding.HLSProcessOpts) []string {
	n := len(opts.Profiles)
	burnIn := opts.SubtitleTrackIndex >= 0 && IsImageSubtitle(opts.SubtitleCodec)
	toneMap := detectToneMap(opts.HDRSupported, opts.Streams)

	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = append(args, "-i", opts.FilePath)

	var filterParts []string
	splitOutputs := make([]string, n)
	for i := range n {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}

	if burnIn {
		if toneMap != "" {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vt]", toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay[vb]", opts.SubtitleTrackIndex),
				fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		} else {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v][0:s:%d]overlay[vb]", opts.SubtitleTrackIndex),
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

	for i, p := range opts.Profiles {
		filterParts = append(filterParts, fmt.Sprintf("[v%d]scale=%d:-2,setsar=1[o%d]", i, p.Width, i))
	}
	args = append(args, "-filter_complex", strings.Join(filterParts, ";"))

	for i, p := range opts.Profiles {
		args = append(args,
			"-map", fmt.Sprintf("[o%d]", i),
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-b:v:%d", i), fmt.Sprintf("%dk", p.VideoBitrate),
			"-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
			fmt.Sprintf("-c:a:%d", i), "aac", "-ac", "2",
			fmt.Sprintf("-b:a:%d", i), fmt.Sprintf("%dk", p.AudioBitrate),
		)
		if opts.AudioNormalize {
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
		"-hls_segment_filename", filepath.Join(opts.OutputDir, "stream_%v_%03d.ts"),
		filepath.Join(opts.OutputDir, "stream_%v.m3u8"),
	)
	return args
}

func detectToneMap(hdrSupported bool, streams []*library.MediaStream) string {
	if hdrSupported {
		return ""
	}
	hdrStreams := make([]hdr.VideoStream, len(streams))
	for i, st := range streams {
		hdrStreams[i] = st
	}
	hdrInfo := hdr.DetectHDR(hdrStreams)
	if hdrInfo.IsHDR {
		return hdr.BuildToneMapFilter(hdrInfo, hdr.DefaultToneMapOptions())
	}
	return ""
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
