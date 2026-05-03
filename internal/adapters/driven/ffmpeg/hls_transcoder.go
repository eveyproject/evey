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

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
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

	// Single-variant outputs (copy + single-profile transcode) write only
	// stream_0.m3u8; we synthesise master.m3u8 manually. Multi-variant
	// transcodes use ffmpeg's -master_pl_name and produce master.m3u8 directly.
	isSingle := h.opts.VideoDecision == "copy" || len(h.opts.Profiles) <= 1

	if isSingle {
		streamPath := filepath.Join(h.opts.OutputDir, "stream_0.m3u8")
		if err := waitForFile(ctx, streamPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for stream_0.m3u8: %w", err)
			h.cancel()
			if err := h.cmd.Wait(); err != nil { slog.Debug("ffmpeg wait after cancel", slog.Any("err", err)) }
			close(h.ready)
			return
		}
		master := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nstream_0.m3u8\n"
		if err := os.WriteFile(filepath.Join(h.opts.OutputDir, "master.m3u8"), []byte(master), 0644); err != nil {
			h.err = fmt.Errorf("write master.m3u8: %w", err)
			h.cancel()
			if err := h.cmd.Wait(); err != nil { slog.Debug("ffmpeg wait after cancel", slog.Any("err", err)) }
			close(h.ready)
			return
		}
		if err := waitForSegment(ctx, h.opts.OutputDir, "stream_0", 10*time.Second, 100*time.Millisecond); err != nil {
			slog.Warn("hls: first segment not yet ready, proceeding anyway", slog.Any("err", err))
		}
	} else {
		masterPath := filepath.Join(h.opts.OutputDir, "master.m3u8")
		if err := waitForFile(ctx, masterPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for master.m3u8: %w", err)
			h.cancel()
			if err := h.cmd.Wait(); err != nil { slog.Debug("ffmpeg wait after cancel", slog.Any("err", err)) }
			close(h.ready)
			return
		}
		if err := waitForSegment(ctx, h.opts.OutputDir, "stream_0", 10*time.Second, 100*time.Millisecond); err != nil {
			slog.Warn("hls: first segment not yet ready, proceeding anyway", slog.Any("err", err))
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
			if err := h.cmd.Process.Kill(); err != nil {
				slog.Warn("ffmpeg kill after timeout failed", slog.Any("err", err))
			}
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

// resolveVideoEncoder maps a domain codec name ("h264"/"hevc"/"h265"/"av1") to
// the ffmpeg encoder name. Empty → libx264 (previous hardcoded default).
func resolveVideoEncoder(codec string) string {
	switch strings.ToLower(codec) {
	case "", "h264", "libx264":
		return "libx264"
	case "hevc", "h265", "libx265":
		return "libx265"
	case "av1", "libaom-av1":
		// libaom-av1 is extremely slow for real-time HLS and may be absent
		// from minimal ffmpeg builds — accept the mapping but callers should
		// avoid requesting av1 unless the build supports it.
		return "libaom-av1"
	}
	return "libx264"
}

// resolveAudioEncoder maps a domain audio codec to the ffmpeg encoder name.
// Empty → aac (previous hardcoded default).
func resolveAudioEncoder(codec string) string {
	switch strings.ToLower(codec) {
	case "", "aac":
		return "aac"
	case "ac3":
		return "ac3"
	case "eac3":
		return "eac3"
	case "opus", "libopus":
		return "libopus"
	}
	return "aac"
}

// resolveAudioChannels returns the -ac value. Zero MaxAudioChannels preserves
// the previous default of 2.
func resolveAudioChannels(maxChannels int) string {
	if maxChannels > 0 {
		return fmt.Sprintf("%d", maxChannels)
	}
	return "2"
}

func BuildFFmpegArgs(opts transcoding.HLSProcessOpts) []string {
	if opts.VideoDecision == "copy" {
		return buildCopyVideoArgs(opts)
	}
	if len(opts.Profiles) <= 1 {
		return buildSingleTranscodeArgs(opts)
	}
	return buildMultiVariantArgs(opts)
}

func needsFMP4Segments(streams []*library.MediaStream) bool {
	for _, s := range streams {
		if s.StreamType == "video" {
			switch strings.ToLower(s.Codec) {
			case "hevc", "h265", "av1", "vp9":
				return true
			}
		}
	}
	return false
}

func buildCopyVideoArgs(opts transcoding.HLSProcessOpts) []string {
	var args []string
	// HLS seek precision: ffmpeg has -accurate_seek enabled by default
	// since 2.1, which makes -ss BEFORE -i frame-accurate as soon as the
	// stream is re-encoded (transcode, not copy). The pre-input -ss is
	// the fast variant (jumps via the index, then decodes from the
	// preceding keyframe up to the target second), so the manifest
	// appears in the m3u8 within ~1s even on 30+ minute seeks.
	//
	// The catch is MPEG-TS muxing: by default ffmpeg shifts PTS by ~1.4s
	// to give the demuxer headroom (PCR initial offset). hls.js then
	// thinks the first segment starts at t=1.4s rather than t=0 and the
	// playback clock drifts off by that amount, which the user
	// experiences as "I clicked 36:41 and landed at ~36:18" (offset +
	// snap_keyframe combination).
	//
	// -muxpreload 0 -muxdelay 0 disables the PTS shift so segments
	// expose their natural starting timestamp (effectively 0). The HLS
	// timeline ends up flush with the source-relative seek.
	//
	// Earlier attempts that were wrong:
	//   - post-input -ss + copy : snap keyframe-after (+0..+5s drift)
	//   - pre-input -ss + force_key_frames + tune zerolatency : -20s
	//   - hybrid two-stage -ss : second -ss treated as output position
	//     by ffmpeg, decoded from file-start of the *post-seek* stream
	//     which gave nonsensical offsets
	if opts.StartSec > 0 {
		// Jellyfin's exact recipe for HLS video transcode (verified
		// against EncodingHelper.cs in master, 2025): pre-input -ss
		// (accurate_seek default) + libx264 + force_key_frames every 5s
		// + copyts/avoid_negative_ts/start_at_zero so the segments
		// preserve source PTS. The client honours the requested seek
		// position because the m3u8 segments carry the matching
		// timestamps. No -noaccurate_seek (that's only for mp4 + copy
		// remux), no -muxpreload/muxdelay (conflicts with -copyts), no
		// -tune zerolatency (was the culprit of the earlier -20s drift).
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
		args = append(args, "-i", opts.FilePath)
		args = append(args,
			"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-pix_fmt", "yuv420p",
			// 1s GOP via -g + -keyint_min so the FIRST keyframe is at most
			// 1s into the output. Combined with -copyts -start_at_zero,
			// hls.js's startPosition:0 lands within ~1s of the requested
			// seek instead of waiting for the first IDR at the ~5s GOP
			// boundary that an expr:gte(t,n_forced*5) policy would produce.
			"-g", "24",
			"-keyint_min", "24",
			"-sc_threshold", "0",
			"-copyts",
			"-avoid_negative_ts", "disabled",
			"-start_at_zero",
		)
	} else {
		args = append(args, "-i", opts.FilePath)
		args = append(args,
			"-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
			"-c:v", "copy",
		)
	}
	if opts.AudioDecision == "transcode" {
		args = append(args,
			"-c:a", resolveAudioEncoder(opts.AudioCodec),
			"-ac", resolveAudioChannels(opts.MaxAudioChannels),
			"-b:a", "192k",
		)
		if opts.AudioNormalize {
			args = append(args, "-af", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
		}
	} else {
		args = append(args, "-c:a", "copy")
	}

	useFMP4 := needsFMP4Segments(opts.Streams)
	args = append(args, "-f", "hls", "-hls_time", "3", "-hls_list_size", "0")
	if useFMP4 {
		args = append(args,
			"-hls_segment_type", "fmp4",
			"-hls_fmp4_init_filename", "stream_0_init.mp4",
			"-hls_segment_filename", filepath.Join(opts.OutputDir, "stream_0_%03d.m4s"),
		)
	} else {
		args = append(args,
			"-hls_segment_filename", filepath.Join(opts.OutputDir, "stream_0_%03d.ts"),
		)
	}
	args = append(args, filepath.Join(opts.OutputDir, "stream_0.m3u8"))
	return args
}

func buildSingleTranscodeArgs(opts transcoding.HLSProcessOpts) []string {
	burnIn := opts.SubtitleTrackIndex >= 0 && IsImageSubtitle(opts.SubtitleCodec) && !opts.ForceSidecarSubs
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
		"-c:v", resolveVideoEncoder(opts.VideoCodec),
		"-profile:v", "high", "-level:v", "4.1",
		"-b:v", fmt.Sprintf("%dk", p.VideoBitrate),
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
	)

	if opts.AudioDecision == "transcode" {
		args = append(args,
			"-c:a", resolveAudioEncoder(opts.AudioCodec),
			"-ac", resolveAudioChannels(opts.MaxAudioChannels),
			"-b:a", fmt.Sprintf("%dk", p.AudioBitrate),
		)
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
	burnIn := opts.SubtitleTrackIndex >= 0 && IsImageSubtitle(opts.SubtitleCodec) && !opts.ForceSidecarSubs
	toneMap := detectToneMap(opts.HDRSupported, opts.Streams)
	vcodec := resolveVideoEncoder(opts.VideoCodec)
	acodec := resolveAudioEncoder(opts.AudioCodec)
	acChannels := resolveAudioChannels(opts.MaxAudioChannels)

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
			fmt.Sprintf("-c:v:%d", i), vcodec,
			fmt.Sprintf("-profile:v:%d", i), "high", fmt.Sprintf("-level:v:%d", i), "4.1",
			fmt.Sprintf("-b:v:%d", i), fmt.Sprintf("%dk", p.VideoBitrate),
			"-preset", "veryfast",
			"-pix_fmt", "yuv420p",
			"-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
			fmt.Sprintf("-c:a:%d", i), acodec, "-ac", acChannels,
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

func waitForSegment(ctx context.Context, dir, streamPrefix string, timeout, interval time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for first segment of %s", streamPrefix)
		case <-ticker.C:
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				name := e.Name()
				if len(name) > len(streamPrefix) && name[:len(streamPrefix)] == streamPrefix && (strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".m4s")) {
					return nil
				}
			}
		}
	}
}
