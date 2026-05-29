package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	slog.Info("hls: ffmpeg start",
		slog.String("media_path", opts.FilePath),
		slog.String("video_decision", opts.VideoDecision),
		slog.String("audio_decision", opts.AudioDecision),
		slog.String("audio_codec", opts.AudioCodec),
		slog.Int("audio_track", opts.AudioTrackIndex),
		slog.String("args", strings.Join(args, " ")))

	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, "ffmpeg", args...)
	// Capture stderr in a ring buffer so we can surface real ffmpeg errors
	// (codec mismatch, channel layout, missing audio encoder, etc.). Without
	// this the audio-only-failure cases are invisible — ffmpeg writes a
	// fatal error on stderr and we'd otherwise just see the consumer's
	// generic 5xx with no clue why.
	stderr := newRingBuffer(8 * 1024)
	cmd.Stderr = stderr

	h := &hlsHandle{
		opts:   opts,
		cmd:    cmd,
		cancel: cancel,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
		stderr: stderr,
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
	stderr *ringBuffer
	paused bool // ffmpeg currently SIGSTOPped (client idle)
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
			if err := h.cmd.Wait(); err != nil {
				slog.Debug("ffmpeg wait after cancel", slog.Any("err", err))
			}
			close(h.ready)
			return
		}
		master := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nstream_0.m3u8\n"
		if err := os.WriteFile(filepath.Join(h.opts.OutputDir, "master.m3u8"), []byte(master), 0644); err != nil {
			h.err = fmt.Errorf("write master.m3u8: %w", err)
			h.cancel()
			if err := h.cmd.Wait(); err != nil {
				slog.Debug("ffmpeg wait after cancel", slog.Any("err", err))
			}
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
			if err := h.cmd.Wait(); err != nil {
				slog.Debug("ffmpeg wait after cancel", slog.Any("err", err))
			}
			close(h.ready)
			return
		}
		if err := waitForSegment(ctx, h.opts.OutputDir, "stream_0", 10*time.Second, 100*time.Millisecond); err != nil {
			slog.Warn("hls: first segment not yet ready, proceeding anyway", slog.Any("err", err))
		}
	}

	close(h.ready)

	if err := h.cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("hls: ffmpeg exited with error",
			slog.Any("err", err),
			slog.String("stderr_tail", h.stderr.String()))
	}
}

// ringBuffer keeps the last N bytes written to it. We use it to capture
// ffmpeg's stderr tail without unbounded memory growth: ffmpeg can spam
// MBs of progress lines, we only care about the last few KB which carry
// the fatal error message on exit.
type ringBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	full bool
	pos  int
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{buf: make([]byte, size), size: size}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	for _, b := range p {
		r.buf[r.pos] = b
		r.pos++
		if r.pos >= r.size {
			r.pos = 0
			r.full = true
		}
	}
	return n, nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return string(r.buf[:r.pos])
	}
	out := make([]byte, 0, r.size)
	out = append(out, r.buf[r.pos:]...)
	out = append(out, r.buf[:r.pos]...)
	return string(out)
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
	// A SIGSTOPped process can't observe ctx cancellation. Always SIGCONT first
	// so the cancel→graceful-exit path completes in milliseconds; otherwise we
	// pay the full 5s timeout and fall through to SIGKILL on every throttled
	// session teardown.
	h.mu.Lock()
	if h.paused && h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Signal(syscall.SIGCONT)
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

// Pause sends SIGSTOP so ffmpeg stops encoding while the client isn't fetching
// segments. With the event-mode playlist nothing can be evicted underneath the
// paused user, but -readrate 1.5 still lets ffmpeg keep encoding ~1.5× real-time
// during a long pause — burning CPU and disk on segments the user may never
// reach. Suspending while idle keeps the playlist stable and the host quiet.
// Idempotent; a no-op once the process has exited.
func (h *hlsHandle) Pause() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.paused || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	if err := h.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		// Process likely already exited (finished/killed) — nothing to suspend.
		return err
	}
	h.paused = true
	return nil
}

// Resume sends SIGCONT to let a paused ffmpeg continue. Idempotent.
func (h *hlsHandle) Resume() error {
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

// HLS playlist mode: event (growing, immutable beginning).
//
// -hls_list_size 0 keeps every segment in the playlist from the start of the
// session. -hls_playlist_type event marks the playlist as EVENT so hls.js
// knows the beginning is stable and will never snap to the live edge
// (liveSyncPosition). This eliminates the ~6-minute drift-jump that occurred
// with the old sliding-window approach (-hls_list_size 90 + delete_segments):
// at -readrate 1.5, ffmpeg encodes 1.5x faster than playback, accumulating a
// 0.5 s/s gap; after ~9 min the oldest segments were deleted and hls.js
// snapped forward to the live edge.
//
// On-disk cost is bounded by HLSSession.stop() which calls os.RemoveAll on
// the OutputDir at session end — no manual segment deletion needed. Worst case
// is bitrate x media_duration per session (~5 GB for a 2h 1080p multivariant).

// hlsSegmentSeconds is the target HLS segment length. It MUST stay in sync with
// the -force_key_frames cadence (hlsForceKeyframesArgs) so ffmpeg emits a
// keyframe exactly at each segment boundary.
const hlsSegmentSeconds = "3"

// deinterlaceFilter deinterlaces only frames the decoder flags as interlaced
// (deint=interlaced) — a no-op on progressive content (verified: passes through
// untouched), so it is safe to apply unconditionally on the transcode path.
// Fixes the horizontal "combing" on interlaced sources (old TV/repacks) that
// would otherwise be encoded field-by-field into progressive frames.
const deinterlaceFilter = "bwdif=deint=interlaced"

// hlsForceKeyframesArgs forces a keyframe exactly every segment. Without it,
// x264's sparse default GOP (~10s) made the HLS muxer wait many seconds for a
// keyframe before it could close the FIRST segment — the dominant cause of the
// multi-second startup delay — and produced irregular segment lengths. The
// expr form is fps-independent. Negligible bitrate cost at a 3s cadence.
var hlsForceKeyframesArgs = []string{"-force_key_frames", "expr:gte(t,n_forced*" + hlsSegmentSeconds + ")"}

// hlsInputReadrate caps ffmpeg's read speed at 1.5× real-time. Even though
// the disk is now bounded by stop() cleanup rather than delete_segments,
// readrate still avoids wasting CPU encoding frames the user will never reach
// (e.g. open a movie, watch 5 minutes, close).
const hlsInputReadrate = "1.5"

// appendInputModifiers prepends -readrate to args before the next -i.
// Caller is responsible for adding -i and the file path after this call.
// Caps the input read rate so ffmpeg stays close to real-time and doesn't
// fill the disk / CPU with buffer the user will never reach.
func appendInputModifiers(args []string) []string {
	return append(args, "-readrate", hlsInputReadrate)
}

// appendHLSOutputArgs writes the HLS muxer arguments shared by every
// builder. list_size 0 + playlist_type event keep all segments available for
// the full session duration; useFMP4 picks the segment container.
// `temp_file` makes ffmpeg write each segment to `<name>.tmp` first, then
// rename atomically on close — so a concurrent reader never sees a partial
// segment (audit indies: Stash pattern, free with this flag on Linux/macOS).
//
// startNumber controls ffmpeg's HLS segment counter (and so the segment
// filenames stream_N_NNN.{ts|m4s}). When the rafale starts mid-media via
// `-ss`, callers must compute startNumber = floor(StartSec/hlsSegmentSeconds)
// so the produced filenames match the absolute manifest indices the VOD
// pipeline cache addresses by. Pass 0 for legacy / non-rafale callers.
func appendHLSOutputArgs(args []string, outputDir string, useFMP4 bool, startNumber int) []string {
	args = append(args,
		"-f", "hls",
		"-hls_time", hlsSegmentSeconds,
		"-hls_list_size", "0",
		"-hls_playlist_type", "event",
		"-hls_flags", "temp_file",
		"-start_number", fmt.Sprintf("%d", startNumber),
	)
	if useFMP4 {
		args = append(args,
			"-hls_segment_type", "fmp4",
			"-hls_fmp4_init_filename", "stream_0_init.mp4",
			"-hls_segment_filename", filepath.Join(outputDir, "stream_0_%03d.m4s"),
		)
	} else {
		args = append(args,
			"-hls_segment_filename", filepath.Join(outputDir, "stream_0_%03d.ts"),
		)
	}
	args = append(args, filepath.Join(outputDir, "stream_0.m3u8"))
	return args
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
		args = appendInputModifiers(args)
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
		args = appendInputModifiers(args)
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

	return appendHLSOutputArgs(args, opts.OutputDir, needsFMP4Segments(opts.Streams), startNumberFor(opts))
}

// startNumberFor maps StartSec onto the segment counter so produced filenames
// match the absolute manifest indices the VOD pipeline addresses. floor()
// because segment N covers media seconds [N*L, (N+1)*L), so a StartSec of
// 153 s with 3 s segments lands inside seg 51 — start_number must be 51.
//
// hlsSegmentSeconds is a const string ("3") rather than a float to stay in
// sync with ffmpeg's -hls_time argument shape. Parsing once is cheap and
// keeps the source of truth in one place.
func startNumberFor(opts transcoding.HLSProcessOpts) int {
	if opts.StartSec <= 0 {
		return 0
	}
	segSec, err := strconv.ParseFloat(hlsSegmentSeconds, 64)
	if err != nil || segSec <= 0 {
		segSec = 3.0
	}
	return int(opts.StartSec / segSec)
}

func buildSingleTranscodeArgs(opts transcoding.HLSProcessOpts) []string {
	burnIn := opts.SubtitleTrackIndex >= 0 && IsImageSubtitle(opts.SubtitleCodec) && !opts.ForceSidecarSubs
	toneMap := detectToneMap(opts.HDRSupported, opts.Streams)

	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = appendInputModifiers(args)
	args = append(args, "-i", opts.FilePath)

	p := opts.Profiles[len(opts.Profiles)-1]

	if burnIn || toneMap != "" {
		var filterParts []string
		switch {
		case burnIn && toneMap != "":
			// deinterlace → tonemap → overlay → scale
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,%s[vt]", deinterlaceFilter, toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay,scale=%d:-2,setsar=1[out]", opts.SubtitleTrackIndex, p.Width),
			)
		case burnIn:
			// deinterlace the video before burning the subtitle overlay on it
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vd]", deinterlaceFilter),
				fmt.Sprintf("[vd][0:s:%d]overlay,scale=%d:-2,setsar=1[out]", opts.SubtitleTrackIndex, p.Width),
			)
		default:
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,%s,scale=%d:-2,setsar=1[out]", deinterlaceFilter, toneMap, p.Width),
			)
		}
		args = append(args, "-filter_complex", strings.Join(filterParts, ";"))
		args = append(args, "-map", "[out]")
	} else {
		args = append(args, "-map", "0:v:0",
			"-vf", fmt.Sprintf("%s,scale=%d:-2,setsar=1", deinterlaceFilter, p.Width),
		)
	}

	args = append(args, "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
		"-c:v", resolveVideoEncoder(opts.VideoCodec),
		"-profile:v", "high", "-level:v", "4.1",
		"-b:v", fmt.Sprintf("%dk", p.VideoBitrate),
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
	)
	args = append(args, hlsForceKeyframesArgs...)

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

	return appendHLSOutputArgs(args, opts.OutputDir, false, startNumberFor(opts))
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
	args = appendInputModifiers(args)
	args = append(args, "-i", opts.FilePath)

	var filterParts []string
	splitOutputs := make([]string, n)
	for i := range n {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}

	// Every branch deinterlaces [0:v] first (no-op on progressive) so interlaced
	// sources don't comb across all variants.
	if burnIn {
		if toneMap != "" {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,%s[vt]", deinterlaceFilter, toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay[vb]", opts.SubtitleTrackIndex),
				fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		} else {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vd]", deinterlaceFilter),
				fmt.Sprintf("[vd][0:s:%d]overlay[vb]", opts.SubtitleTrackIndex),
				fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		}
	} else {
		if toneMap != "" {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,%s[vt]", deinterlaceFilter, toneMap),
				fmt.Sprintf("[vt]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		} else {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,split=%d%s", deinterlaceFilter, n, strings.Join(splitOutputs, "")),
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
	// Force aligned keyframes at the segment cadence across every variant — both
	// to fix slow/irregular first-segment startup and to keep ABR-switchable
	// segment boundaries identical between renditions.
	args = append(args, hlsForceKeyframesArgs...)

	var streamMap []string
	for i := range n {
		streamMap = append(streamMap, fmt.Sprintf("v:%d,a:%d", i, i))
	}
	args = append(args, "-var_stream_map", strings.Join(streamMap, " "))

	// Multi-variant master playlist — same event-mode flags as the single-variant
	// builders: list_size 0 keeps the full growing playlist, playlist_type event
	// prevents hls.js from snapping to the live edge. Cleanup is handled by
	// HLSSession.stop() → os.RemoveAll(OutputDir) at session end.
	args = append(args,
		"-master_pl_name", "master.m3u8",
		"-f", "hls",
		"-hls_time", hlsSegmentSeconds,
		"-hls_list_size", "0",
		"-hls_playlist_type", "event",
		"-hls_flags", "temp_file",
		"-start_number", fmt.Sprintf("%d", startNumberFor(opts)),
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
