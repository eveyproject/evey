package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// Per-variant HW: one single-output ffmpeg per ABR rung behind a composite
	// handle, instead of one multi-output process. Matches how Jellyfin/Plex
	// drive HW — the multi-encode-in-one-process graph crashes fragile VA
	// drivers (and the WSL d3d12 shim) and a single rung's failure shouldn't
	// take the others down. Software keeps the single-process multi-variant
	// graph (decode once, encode N — efficient and crash-free on libx264).
	if opts.PerVariantHW && opts.HWAccel != "" && opts.HWAccel != transcoding.None && len(opts.Profiles) > 1 {
		return startPerVariant(ctx, opts)
	}
	h, err := startOne(ctx, opts, false)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// needsCopyRebase: EVERY copy fMP4 session publishes through the rebase pump.
// Seek sessions (StartNumber > 0) need their session-relative tfdt/sidx
// shifted to the absolute timeline (movenc always writes dts − first_dts; no
// CLI flag yields absolute time — see fmp4_rebase.go). Cold sessions are
// absolute by construction (deltaMs 0, parse-only) but still route through
// the pump so the timeline TRIPWIRE (verifyCopySegmentTimeline) covers them:
// the historical numbering-drift bug lived precisely on the cold path.
// Transcode rungs share the latent base mismatch but are paced (no rewrite
// race) — tracked separately.
func needsCopyRebase(o transcoding.HLSProcessOpts) bool {
	return o.VideoDecision == "copy" && o.UseFMP4
}

// startOne spawns a single ffmpeg process for opts and returns its handle.
// noMaster suppresses the synthesised one-rung master.m3u8 (set for the
// sub-processes of a per-variant composite).
func startOne(ctx context.Context, opts transcoding.HLSProcessOpts, noMaster bool) (*hlsHandle, error) {
	// Copy-seek sessions write into a private subdir; runRebasePump shifts
	// their fMP4 timestamps to the plan's absolute StartSec and atomically
	// renames into the cache dir — the rename is the publish point
	// SegmentCache.IsReady observes, so readers never see a pre-rebase file.
	pubDir := opts.OutputDir
	rebase := needsCopyRebase(opts)
	if rebase {
		priv := filepath.Join(pubDir, fmt.Sprintf(".rebase_%d_%d", opts.StartNumber, time.Now().UnixNano()))
		if err := os.MkdirAll(priv, 0o755); err != nil {
			return nil, fmt.Errorf("copy rebase dir: %w", err)
		}
		opts.OutputDir = priv
	}
	args := BuildFFmpegArgs(opts)
	slog.Info("hls: ffmpeg start",
		slog.String("media_path", opts.FilePath),
		slog.String("video_decision", opts.VideoDecision),
		slog.String("audio_decision", opts.AudioDecision),
		slog.String("audio_codec", opts.AudioCodec),
		slog.Int("audio_track", opts.AudioTrackIndex),
		slog.Int("variant", opts.VariantIndex),
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
		opts:     opts,
		cmd:      cmd,
		cancel:   cancel,
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
		stderr:   stderr,
		noMaster: noMaster,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		if rebase {
			_ = os.RemoveAll(opts.OutputDir)
		}
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	go h.run(processCtx)
	if rebase {
		deltaMs := int64(opts.StartSec*1000 + 0.5)
		go runRebasePump(h.done, processCtx.Done(), opts.OutputDir, pubDir, deltaMs, opts.VariantIndex, opts.ExpectedSegmentStartMs)
	}
	return h, nil
}

// startPerVariant spawns one single-output ffmpeg per profile (each writing its
// own stream_<i>_* rung) and returns a compositeHandle that fans Stop/Pause/
// Resume to all and aggregates Done/Failure. The composite ctx cancel kills
// every rung at once.
func startPerVariant(ctx context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	procCtx, cancel := context.WithCancel(ctx)
	c := &compositeHandle{cancel: cancel, done: make(chan struct{})}
	for i := range opts.Profiles {
		sub := opts
		sub.Profiles = []transcoding.HLSProfile{opts.Profiles[i]}
		sub.VariantIndex = i
		sub.PerVariantHW = false // the sub IS a single rung; don't recurse
		h, err := startOne(procCtx, sub, true)
		if err != nil {
			cancel()
			for _, s := range c.subs {
				_ = s.Stop() //nolint:errcheck // best-effort teardown; we return the start error below
			}
			return nil, fmt.Errorf("per-variant ffmpeg start (rung %d): %w", i, err)
		}
		c.subs = append(c.subs, h)
	}
	go c.reap()
	return c, nil
}

// compositeHandle presents N single-output ffmpeg sub-handles (one per ABR
// rung) as one HLSProcessHandle. Done closes when ALL rungs exit; Failure
// surfaces a HW-capability class from any rung (so the orchestrator demotes the
// host) else the first non-None; Stop/Pause/Resume fan out to every rung.
type compositeHandle struct {
	subs    []*hlsHandle
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	stopped bool
}

func (c *compositeHandle) reap() {
	for _, s := range c.subs {
		<-s.Done()
	}
	close(c.done)
}

func (c *compositeHandle) WaitReady(ctx context.Context) error {
	for _, s := range c.subs {
		if err := s.WaitReady(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *compositeHandle) Stop() error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return nil
	}
	c.stopped = true
	c.mu.Unlock()
	c.cancel()
	var wg sync.WaitGroup
	errs := make([]error, len(c.subs))
	for i, s := range c.subs {
		wg.Add(1)
		go func(idx int, h *hlsHandle) { defer wg.Done(); errs[idx] = h.Stop() }(i, s)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (c *compositeHandle) Done() <-chan struct{} { return c.done }

func (c *compositeHandle) Err() error {
	for _, s := range c.subs {
		if e := s.Err(); e != nil {
			return e
		}
	}
	return nil
}

func (c *compositeHandle) Failure() transcoding.FailureClass {
	first := transcoding.FailureNone
	for _, s := range c.subs {
		f := s.Failure()
		if f.IsHWCapability() {
			return f // any rung's HW failure → demote the host
		}
		if first == transcoding.FailureNone {
			first = f
		}
	}
	return first
}

func (c *compositeHandle) Pause() error {
	var errs []error
	for _, s := range c.subs {
		errs = append(errs, s.Pause())
	}
	return errors.Join(errs...)
}

func (c *compositeHandle) Resume() error {
	var errs []error
	for _, s := range c.subs {
		errs = append(errs, s.Resume())
	}
	return errors.Join(errs...)
}

type hlsHandle struct {
	opts    transcoding.HLSProcessOpts
	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	ready   chan struct{}
	done    chan struct{}
	err     error
	exitErr error // raw ffmpeg exit error, for failure classification (guarded by mu)
	stderr  *ringBuffer
	paused  bool // ffmpeg currently SIGSTOPped (client idle)
	// noMaster suppresses the synthesised single-rung master.m3u8. Set on the
	// sub-processes of a per-variant composite handle (the VOD pipeline serves a
	// precomputed master, and N siblings must not each clobber master.m3u8 with
	// a one-rung playlist).
	noMaster bool
}

func (h *hlsHandle) setExitErr(err error) {
	h.mu.Lock()
	h.exitErr = err
	h.mu.Unlock()
}

func (h *hlsHandle) run(ctx context.Context) {
	defer close(h.done)

	// Single-variant outputs (copy + single-profile transcode) write only
	// stream_0.m3u8; we synthesise master.m3u8 manually. Multi-variant
	// transcodes use ffmpeg's -master_pl_name and produce master.m3u8 directly.
	isSingle := h.opts.VideoDecision == "copy" || len(h.opts.Profiles) <= 1

	if isSingle {
		streamName := fmt.Sprintf("stream_%d", h.opts.VariantIndex)
		streamPath := filepath.Join(h.opts.OutputDir, streamName+".m3u8")
		if err := waitForFile(ctx, streamPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for %s.m3u8: %w", streamName, err)
			h.cancel()
			we := h.cmd.Wait()
			h.setExitErr(we)
			if we != nil {
				slog.Debug("ffmpeg wait after cancel", slog.Any("err", we))
			}
			close(h.ready)
			return
		}
		// Standalone single-variant handles synthesise the one-rung master; a
		// per-variant composite's siblings set noMaster (the VOD pipeline serves
		// a precomputed master, and N siblings must not clobber master.m3u8).
		if !h.noMaster {
			master := fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\n%s.m3u8\n", streamName)
			if err := os.WriteFile(filepath.Join(h.opts.OutputDir, "master.m3u8"), []byte(master), 0644); err != nil {
				h.err = fmt.Errorf("write master.m3u8: %w", err)
				h.cancel()
				we := h.cmd.Wait()
				h.setExitErr(we)
				if we != nil {
					slog.Debug("ffmpeg wait after cancel", slog.Any("err", we))
				}
				close(h.ready)
				return
			}
		}
		if err := waitForSegment(ctx, h.opts.OutputDir, streamName, 10*time.Second, 100*time.Millisecond); err != nil {
			slog.Warn("hls: first segment not yet ready, proceeding anyway", slog.Any("err", err))
		}
	} else {
		masterPath := filepath.Join(h.opts.OutputDir, "master.m3u8")
		if err := waitForFile(ctx, masterPath, 30*time.Second, 100*time.Millisecond); err != nil {
			h.err = fmt.Errorf("waiting for master.m3u8: %w", err)
			h.cancel()
			we := h.cmd.Wait()
			h.setExitErr(we)
			if we != nil {
				slog.Debug("ffmpeg wait after cancel", slog.Any("err", we))
			}
			close(h.ready)
			return
		}
		if err := waitForSegment(ctx, h.opts.OutputDir, "stream_0", 10*time.Second, 100*time.Millisecond); err != nil {
			slog.Warn("hls: first segment not yet ready, proceeding anyway", slog.Any("err", err))
		}
	}

	close(h.ready)

	we := h.cmd.Wait()
	h.setExitErr(we)
	if we != nil && ctx.Err() == nil {
		slog.Warn("hls: ffmpeg exited with error",
			slog.Any("err", we),
			slog.String("stderr_tail", h.stderr.String()))
	}
}

// Failure classifies why the process exited, for the level-2 HW safety net. It
// is only meaningful after Done() has closed; before that exitErr is nil and it
// reports FailureNone. Reads the raw exit error + the captured stderr tail.
func (h *hlsHandle) Failure() transcoding.FailureClass {
	h.mu.Lock()
	e := h.exitErr
	h.mu.Unlock()
	return ClassifyHWFailure(e, h.stderr.String())
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
		// Failure here only matters if the process has already exited,
		// in which case the Stop path below treats the missing process
		// as already-stopped — nothing to recover.
		if err := h.cmd.Process.Signal(syscall.SIGCONT); err != nil {
			_ = err // best-effort wake before stop
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

// IsImageSubtitle re-exports the domain's single source of truth (the adapter
// may import the domain — what it must not import is the application layer,
// which is why this used to be a hand-synced duplicate).
func IsImageSubtitle(codec string) bool {
	return transcoding.IsImageSubtitle(codec)
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

// softwareProfileLevelArgs returns the -profile:v / -level:v args for a software
// encoder. "high"/"4.1" are H.264 (libx264) values: libx265 rejects them outright
// ("x265 [error]: unknown profile <high>" → the encoder fails to open and the
// whole session dies) and libaom-av1 uses different profile/level strings, so
// non-libx264 encoders get nothing and derive the profile from the pixel format.
// variantIdx < 0 emits the single-output form (-profile:v); else the per-rung
// -profile:v:N form used by the multi-variant builder. Reachable via a client
// PreferredVideoCodec=hevc on a software path (no HW HEVC) — found by audit.
func softwareProfileLevelArgs(encoder string, variantIdx int) []string {
	if encoder != "libx264" {
		return nil
	}
	if variantIdx < 0 {
		return []string{"-profile:v", "high", "-level:v", "4.1"}
	}
	return []string{
		fmt.Sprintf("-profile:v:%d", variantIdx), "high",
		fmt.Sprintf("-level:v:%d", variantIdx), "4.1",
	}
}

// normalizeVideoCodec collapses the domain codec aliases to the canonical
// base used to build hardware encoder names ("h264"/"hevc"/"av1"). Empty
// codec defaults to h264 (matches resolveVideoEncoder's libx264 default).
// Returns "" for anything unrecognised so callers fall back to software.
func normalizeVideoCodec(codec string) string {
	switch strings.ToLower(codec) {
	case "", "h264", "libx264":
		return "h264"
	case "hevc", "h265", "libx265":
		return "hevc"
	case "av1", "libaom-av1":
		return "av1"
	}
	return ""
}

// videoEncoderFor maps a domain codec + hardware backend to the ffmpeg
// encoder name. accel None/empty/invalid → software (resolveVideoEncoder).
// When a backend is requested but the codec has no hardware encoder for it
// (unrecognised codec), it falls back to software so the command stays valid
// — defensive only: the wiring layer already gates HWAccel on per-codec
// capability probing (HWDetector.SupportsCodec), so a set HWAccel is trusted.
func videoEncoderFor(codec string, accel transcoding.HWAccelType) string {
	if accel == "" || accel == transcoding.None || !accel.IsValid() {
		return resolveVideoEncoder(codec)
	}
	base := normalizeVideoCodec(codec)
	if base == "" {
		return resolveVideoEncoder(codec)
	}
	switch accel {
	case transcoding.NVENC:
		return base + "_nvenc"
	case transcoding.VAAPI:
		return base + "_vaapi"
	case transcoding.QSV:
		return base + "_qsv"
	}
	return resolveVideoEncoder(codec)
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

// scaledAudioBitrateKbps scales the ladder's per-rung audio bitrate (sized
// for stereo) to the transcode's channel target: ~64 kbps per channel beyond
// stereo (5.1 → 384k), capped at 512k. Deterministic in (base, maxChannels) —
// both hashed — so a given fingerprint always yields the same audio bytes.
func scaledAudioBitrateKbps(base int64, maxChannels int) int64 {
	if maxChannels <= 2 {
		return base
	}
	scaled := int64(maxChannels) * 64
	if scaled < base {
		return base
	}
	if scaled > 512 {
		return 512
	}
	return scaled
}

// resolveAudioChannels returns the -ac value. Zero MaxAudioChannels preserves
// the previous default of 2.
func resolveAudioChannels(maxChannels int) string {
	if maxChannels > 0 {
		return fmt.Sprintf("%d", maxChannels)
	}
	return "2"
}

// hasAudio reports whether the source has a selectable audio track. The wiring
// layer signals "no audio" with AudioTrackIndex < 0 (rare: a silent/video-only
// media). When false the builders emit a video-only ladder — no -map 0:a, no
// audio encode args, and the var_stream_map drops the audio rendition — so
// ffmpeg doesn't abort with "Invalid argument" on a missing 0:a:N stream.
func hasAudio(opts transcoding.HLSProcessOpts) bool {
	return opts.AudioTrackIndex >= 0
}

// audioOutputArgsSingle returns the audio map + single-stream encode/copy args
// for the copy and single-variant transcode builders, or nil when the source
// has no audio. Honours the copy-vs-transcode decision and optional loudnorm.
func audioOutputArgsSingle(opts transcoding.HLSProcessOpts, bitrateKbps int64) []string {
	if !hasAudio(opts) {
		return nil
	}
	a := []string{"-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex)}
	if opts.AudioDecision == "transcode" {
		a = append(a,
			"-c:a", resolveAudioEncoder(opts.AudioCodec),
			"-ac", resolveAudioChannels(opts.MaxAudioChannels),
			"-b:a", fmt.Sprintf("%dk", scaledAudioBitrateKbps(bitrateKbps, opts.MaxAudioChannels)),
		)
		if opts.AudioNormalize {
			a = append(a, "-filter:a:0", "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
		}
	} else {
		a = append(a, "-c:a", "copy")
	}
	return a
}

// audioVariantArgs returns the per-variant audio map + transcode args for the
// multi-variant builders (software and hardware), or nil when the source has
// no audio. Multi-variant always re-encodes audio (one AAC rendition per rung).
func audioVariantArgs(opts transcoding.HLSProcessOpts, i int, bitrateKbps int64) []string {
	if !hasAudio(opts) {
		return nil
	}
	a := []string{
		"-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex),
		fmt.Sprintf("-c:a:%d", i), resolveAudioEncoder(opts.AudioCodec),
		"-ac", resolveAudioChannels(opts.MaxAudioChannels),
		fmt.Sprintf("-b:a:%d", i), fmt.Sprintf("%dk", scaledAudioBitrateKbps(bitrateKbps, opts.MaxAudioChannels)),
	}
	if opts.AudioNormalize {
		a = append(a, fmt.Sprintf("-filter:a:%d", i), "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
	}
	return a
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
// (deint=interlaced), at the SOURCE frame rate (mode=send_frame). bwdif's
// DEFAULT mode is send_field, which emits one output frame per FIELD — doubling
// the frame rate (e.g. 24→48 fps) even on progressive content, because the rate
// doubling is independent of deint=interlaced (that flag only gates the
// deinterlace *algorithm*, not the field-to-frame expansion). Applied unconditionally
// at the head of the filter chain, send_field silently ran scale + tonemap + the
// x264 encode at 2× the frames: a 4K HEVC10 HDR transcode measured speed=0.90×
// realtime on an old iGPU box (frame=10798 over 224s = 48 fps, dup≈half) — sub-realtime,
// so playback buffered indefinitely with no lower rung to fall back to.
// send_frame keeps the combing fix for genuinely interlaced sources (old TV /
// repacks) at their native rate and is a true 1:1 no-op on progressive content.
const deinterlaceFilter = "bwdif=mode=send_frame:deint=interlaced"

// deinterlaceFilterFor returns the software deinterlacer for these opts. The
// default flag-gated bwdif (deint=interlaced) is a no-op on progressive
// content; when the idet content detection flagged the source interlaced
// despite its container saying otherwise (DeinterlaceAll), it switches to
// deint=all so every frame is deinterlaced — the only way to clear the
// combing a lying flag hides. send_frame still keeps the source frame rate.
func deinterlaceFilterFor(opts transcoding.HLSProcessOpts) string {
	if opts.DeinterlaceAll {
		return "bwdif=mode=send_frame:deint=all"
	}
	return deinterlaceFilter
}

// scaleFilterFor builds the software scale filter for these opts, anamorphic-
// aware. Square-pixel sources keep the historical "scale=W:-2,setsar=1" (even
// height, square output). For an anamorphic source (SourceSAR num != den) the
// output height is computed from the DISPLAY aspect so the picture isn't
// squashed: oh = W·ih·den/(iw·num), rounded to an even value. A 720x576
// DAR-16:9 PAL DVD (SAR 64:45) targeting 640 wide becomes 640x360, not the
// 640x512 the storage-ratio scale would produce. setsar=1 bakes square pixels
// for the encoder either way.
func scaleFilterFor(opts transcoding.HLSProcessOpts, width int) string {
	if num, den, ok := parseAspect(opts.SourceSAR); ok && num != den {
		return fmt.Sprintf("scale=%d:'trunc(%d*ih*%d/(iw*%d)/2)*2',setsar=1", width, width, den, num)
	}
	return fmt.Sprintf("scale=%d:-2,setsar=1", width)
}

// parseAspect parses an "N:D" aspect string into its integer parts. Returns
// ok=false for empty, malformed, or zero-denominator values.
func parseAspect(s string) (num, den int, ok bool) {
	n, d, found := strings.Cut(strings.TrimSpace(s), ":")
	if !found {
		return 0, 0, false
	}
	num, err1 := strconv.Atoi(n)
	den, err2 := strconv.Atoi(d)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, 0, false
	}
	return num, den, true
}

// softwareCRF is the x264 quality target for the software ABR ladder. Paired
// with per-rung -maxrate/-bufsize ("capped CRF"): CRF fixes the perceptual
// quality so easy scenes spend fewer bits than the rung's bitrate (bandwidth
// saved, steady quality), while maxrate caps complex scenes at the bitrate the
// manifest advertises (still ABR-safe — a peak can't blow past what the player
// budgeted). 23 is the x264 sane default. HW encoders ignore CRF (rate-control
// differs), so this is the software path only.
const softwareCRF = "23"

// hlsForceKeyframesArgs forces a keyframe exactly every segment. Without it,
// x264's sparse default GOP (~10s) made the HLS muxer wait many seconds for a
// keyframe before it could close the FIRST segment — the dominant cause of the
// multi-second startup delay — and produced irregular segment lengths. The
// expr form is fps-independent. Negligible bitrate cost at a 3s cadence.
var hlsForceKeyframesArgs = []string{"-force_key_frames", "expr:gte(t,n_forced*" + hlsSegmentSeconds + ")"}

// appendInputModifiers is the hook for pre-`-i` flags. It emits `-readrate N`
// (input pacing: read the source at N× real-time) ONLY when readRate > 0.
//
// Pacing is per-producer, not global. The multi-rafale HW orchestrator leaves
// readRate=0 and burns CPU on segments 0-N until its adaptive throttle (SIGSTOP
// when the client is far ahead, SIGCONT on CMCD bs=1) kicks in — removing the
// cap there fixed the cold-start it used to cripple (-readrate 1.5 made the
// first 4K segment land over hls.js's per-fragment timeout). The single
// long-lived SOFTWARE producer sets readRate≈1.5: with ONE sequential encoder
// and a client that waits (not abort-and-probe), pacing keeps ffmpeg near
// wall-clock so it never races the whole film ahead of the viewer. readRate=0
// keeps the output byte-identical to today.
func appendInputModifiers(args []string, readRate float64) []string {
	if readRate > 0 {
		args = append(args, "-readrate", fmt.Sprintf("%g", readRate))
	}
	return args
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
// hlsTimeFor returns the -hls_time value. The COPY rung always runs "0" — split
// at EVERY keyframe — because that is the only muxer split rule that is
// deterministic AND independent of where the session started. With any
// hls_time > 0 the muxer splits on an absolute n×hls_time grid relative to the
// SESSION's first packet (not "duration since the previous cut"): on long-GOP
// sources the grid lags until every keyframe qualifies anyway, and a `-ss`
// session's grid is offset from the cold session's, so two sessions covering
// the same region cut DIFFERENT segments under the same cache filenames
// (measured in prod: segment numbering drifted minutes from the playlist).
// `-hls_time 0` + the every-keyframe CopyPlanner plan keep ffmpeg and the
// precalculated manifest byte-aligned for every session (verified against
// jellyfin-ffmpeg7, cold + seek, dense and long-GOP sources). The transcode
// path keeps the default: its encoder only EMITS keyframes on the forced
// n×3s grid, so grid-splitting and the fixed manifest coincide exactly.
func hlsTimeFor(opts transcoding.HLSProcessOpts) string {
	if opts.VideoDecision == "copy" {
		return "0"
	}
	return hlsSegmentSeconds
}

func appendHLSOutputArgs(args []string, outputDir string, useFMP4 bool, hlsTime string, startNumber, variantIdx int) []string {
	args = append(args,
		"-f", "hls",
		"-hls_time", hlsTime,
		"-hls_list_size", "0",
		"-hls_playlist_type", "event",
		"-hls_flags", "temp_file",
		"-start_number", fmt.Sprintf("%d", startNumber),
	)
	if useFMP4 {
		args = append(args,
			"-hls_segment_type", "fmp4",
			"-hls_fmp4_init_filename", fmt.Sprintf("stream_%d_init.mp4", variantIdx),
			"-hls_segment_filename", filepath.Join(outputDir, fmt.Sprintf("stream_%d_%%03d.m4s", variantIdx)),
		)
	} else {
		args = append(args,
			"-hls_segment_filename", filepath.Join(outputDir, fmt.Sprintf("stream_%d_%%03d.ts", variantIdx)),
		)
	}
	args = append(args, filepath.Join(outputDir, fmt.Sprintf("stream_%d.m3u8", variantIdx)))
	return args
}

// needsFMP4Segments returns true to mux into fMP4 (`.m4s` + init.mp4)
// rather than MPEG-TS (`.ts`). fMP4 is the project default since Apple
// blessed it as the CMAF container in WWDC 2024; segments are ~10-30%
// smaller than TS for the same bitrate (no 188-byte packet overhead),
// the init segment carries the codec config once instead of repeating
// it in every TS packet, and switching to CMAF later (one encode serving
// HLS + DASH) requires fMP4 anyway. Every browser shipping hls.js or
// Safari natif in the last 5 years supports it.
//
// The `streams` arg is no longer consulted — historically we forced
// fMP4 only for HEVC/AV1/VP9 sources because the TS muxer doesn't carry
// them cleanly. With fMP4 as the default the check is moot. Kept the
// signature in case we ever need a per-stream opt-out hook.
func needsFMP4Segments(streams []*library.MediaStream) bool {
	_ = streams
	return true
}

// appendCopyPacingBurst emits `-readrate_initial_burst` (ffmpeg ≥ 6.1, present
// in the bundled jellyfin-ffmpeg7) right after a copy session's `-readrate`.
// The copy rung paces its input (see SoftwareProducer.copyReadRate) to bound
// the muxer-side A/V skew a free-running copy develops: video `-c:v copy`
// packets cost ~0 while the DTS/AC-3→AAC chain is CPU-bound, so on a weak host
// an UNPACED session lets video race ahead until the muxer's 10 s
// max_interleave_delta force-flushes — measured in prod as audio landing up to
// ~8.3 s behind video across segment files (the audible "son décalé" desync).
// Pacing bounds that skew; the initial burst keeps startup/seek instant by
// reading the first N seconds at full speed before the rate cap kicks in.
func appendCopyPacingBurst(args []string, readRate float64) []string {
	if readRate <= 0 {
		return args
	}
	return append(args, "-readrate_initial_burst", "30")
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
	switch {
	case opts.StartSec > 0 && opts.CopyOnSeek:
		// COPY RUNG mid-stream start: input-seek + -c:v copy. StartSec is a plan
		// keyframe boundary so pre-input -ss lands exactly, and copying preserves
		// the SOURCE avcC. The re-encode branch below (libx264) emitted a Baseline
		// init that OVERWROTE the shared High-profile init → every copied segment
		// then decoded against the wrong SPS/PPS = scrambled colour-band frames.
		//
		// -avoid_negative_ts make_zero (NOT -copyts): the fMP4 init segment is
		// SHARED per variant (stream_N_init.mp4) — every session, seek or not,
		// overwrites it and the player decodes ALL segments against whichever init
		// it loaded. -copyts preserved the absolute source PTS, which fMP4 records
		// as an EDIT LIST in the init (elst media_time = StartSec). A seek session
		// thus wrote a NON-canonical init that presentation-shifted EVERY segment —
		// including the t=0 ones from the cold session — by +StartSec. Proven in a
		// jellyfin-ffmpeg7 harness: cross-decoding a t=0 segment against a 12s-seek
		// init reports start_time=12.0 (a 12 s desync / wrong seek landing).
		// make_zero resets each segment to tfdt=0 and keeps the init BYTE-IDENTICAL
		// regardless of seek offset, so the shared init stays canonical. Segment
		// position comes from the VOD playlist (StartNumber + #EXTINF), exactly like
		// the transcode rung whose segments are also tfdt=0.
		//
		// -noaccurate_seek: the WHOLE session must start at the seek LANDING,
		// audio included. The copied video necessarily starts at the landed
		// keyframe, and the session's bookkeeping (StartSec/StartNumber/tfdt
		// rebase) is set to that landing — but ffmpeg's default accurate-seek
		// DECODES AND DISCARDS the audio between the landing and the -ss target
		// while the muxer still timestamps the first kept audio frame at the
		// landing. Content and labels then disagree by (target − landing): the
		// audio plays EARLY by up to a full GOP (measured: session audio content
		// started at the 1351.33s target but carried the 1350.52s landing label
		// → the deep-seek "son décalé" desync, while start-of-file playback was
		// clean). With -noaccurate_seek the decoded audio from the landing is
		// kept and encoded, so both tracks start at the landing, labels match
		// content, and the first segment carries audio immediately (the
		// "audio one segment-file late" artefact was the discarded-trim gap).
		args = append(args, "-noaccurate_seek")
		// -ss MUST be the exact timestamp the seek-landing probe ran against
		// (SeekTargetSec): probe(T) == landing(T) by construction, and the
		// session's bookkeeping (StartSec/StartNumber = the landing) only holds
		// for that same T. Re-deriving -ss from the landed StartSec would land
		// one keyframe earlier AGAIN (ffmpeg CLI's ~130 ms DTS-heuristic seek
		// offset on B-frame sources). Fallback (no probe ran): the nudged
		// StartSec, correct for dense-cue, no-B-frame sources.
		target := opts.SeekTargetSec
		if target <= 0 {
			target = opts.StartSec + transcoding.CopySeekNudgeSec
		}
		args = append(args, "-ss", fmt.Sprintf("%.3f", target))
		args = appendInputModifiers(args, opts.ReadRate)
		args = appendCopyPacingBurst(args, opts.ReadRate)
		args = append(args, "-i", opts.FilePath, "-map", "0:v:0")
		if hasAudio(opts) {
			args = append(args, "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex))
		}
		args = append(args, "-c:v", "copy", "-avoid_negative_ts", "make_zero")
		args = append(args, copyVideoTagArgs(opts)...)
	case opts.StartSec > 0:
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
		args = appendInputModifiers(args, opts.ReadRate)
		args = append(args, "-i", opts.FilePath)
		args = append(args, "-map", "0:v:0")
		if hasAudio(opts) {
			args = append(args, "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex))
		}
		args = append(args,
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
	default:
		args = appendInputModifiers(args, opts.ReadRate)
		args = appendCopyPacingBurst(args, opts.ReadRate)
		args = append(args, "-i", opts.FilePath)
		args = append(args, "-map", "0:v:0")
		if hasAudio(opts) {
			args = append(args, "-map", fmt.Sprintf("0:a:%d", opts.AudioTrackIndex))
		}
		args = append(args, "-c:v", "copy")
		args = append(args, copyVideoTagArgs(opts)...)
	}
	if hasAudio(opts) {
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
	}

	return appendHLSOutputArgs(args, opts.OutputDir, needsFMP4Segments(opts.Streams), hlsTimeFor(opts), startNumberFor(opts), opts.VariantIndex)
}

// copyVideoTagArgs returns the sample-entry tag override for a video-copy of
// this source. HEVC must be muxed as 'hvc1' (parameter sets in the sample
// entry): ffmpeg's fMP4 default is 'hev1', which Safari/AVPlayer refuse even
// when the master's CODECS attribute says hvc1 — the attribute and the actual
// sample entry must agree. H.264 needs no override (avc1 is the default), and
// the seek-re-encode branch (libx264) never calls this.
func copyVideoTagArgs(opts transcoding.HLSProcessOpts) []string {
	if strings.EqualFold(strings.TrimSpace(opts.SourceVideoCodec), "hevc") {
		return []string{"-tag:v", "hvc1"}
	}
	return nil
}

// startNumberFor maps StartSec onto the segment counter so produced filenames
// match the absolute manifest indices the VOD pipeline addresses. floor()
// because segment N covers media seconds [N*L, (N+1)*L), so a StartSec of
// 153 s with 3 s segments lands inside seg 51 — start_number must be 51.
//
// COPY MODE: segments are keyframe-aligned and VARIABLE length, so StartSec/L is
// NOT the segment index (a seek to plan-segment K has StartSec = the keyframe
// time of K, which floor()s to some other number). The caller passes the
// authoritative absolute index in opts.StartNumber; honour it directly, or the
// seek-spawned encoder writes stream_N_<wrong>.m4s and the player waits forever
// on stream_N_<K>.m4s (the deep-seek "stuck on loading" bug).
//
// hlsSegmentSeconds is a const string ("3") rather than a float to stay in
// sync with ffmpeg's -hls_time argument shape. Parsing once is cheap and
// keeps the source of truth in one place.
func startNumberFor(opts transcoding.HLSProcessOpts) int {
	if opts.VideoDecision == "copy" {
		return opts.StartNumber
	}
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
	p := opts.Profiles[len(opts.Profiles)-1]
	// Fold the downscale into the tonemap for the plain HDR path so the heavy
	// float pipeline runs at the rung width, not the 4K source. The burn-in path
	// keeps a full-resolution tonemap (overlay at a fixed size, then scale).
	dstWidth := 0
	if !burnIn {
		dstWidth = p.Width
	}
	toneMap := resolveToneMap(opts, dstWidth)
	if hwUsable(opts, burnIn, toneMap) {
		return buildSingleTranscodeHWArgs(opts)
	}

	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = appendInputModifiers(args, opts.ReadRate)
	args = append(args, "-i", opts.FilePath)

	deint := deinterlaceFilterFor(opts)
	if burnIn || toneMap != "" {
		var filterParts []string
		switch {
		case burnIn && toneMap != "":
			// deinterlace → tonemap → overlay → scale
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,%s[vt]", deint, toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay,%s[out]", opts.SubtitleTrackIndex, scaleFilterFor(opts, p.Width)),
			)
		case burnIn:
			// deinterlace the video before burning the subtitle overlay on it
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vd]", deint),
				fmt.Sprintf("[vd][0:s:%d]overlay,%s[out]", opts.SubtitleTrackIndex, scaleFilterFor(opts, p.Width)),
			)
		default:
			// tonemap already downscaled to p.Width (folded into its linearizing
			// zscale), so no trailing scale — that 4× cut in tonemapped pixels is
			// the fix that keeps a weak host real-time on 4K HDR sources. (HDR and
			// anamorphic SD are disjoint in practice, so no SAR correction here.)
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,%s,setsar=1[out]", deint, toneMap),
			)
		}
		args = append(args, "-filter_complex", strings.Join(filterParts, ";"))
		args = append(args, "-map", "[out]")
	} else {
		args = append(args, "-map", "0:v:0",
			"-vf", fmt.Sprintf("%s,%s", deint, scaleFilterFor(opts, p.Width)),
		)
	}

	enc := resolveVideoEncoder(opts.VideoCodec)
	args = append(args, "-c:v", enc)
	args = append(args, softwareProfileLevelArgs(enc, -1)...) // h264-only; x265/av1 pick their own
	args = append(args,
		// Capped CRF (see softwareCRF): quality-targeted with the rung bitrate as a
		// hard ceiling, instead of a plain -b:v average.
		"-crf", softwareCRF,
		"-maxrate", fmt.Sprintf("%dk", p.VideoBitrate),
		"-bufsize", fmt.Sprintf("%dk", p.VideoBitrate*2),
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
	)
	args = append(args, hlsForceKeyframesArgs...)
	args = append(args, audioOutputArgsSingle(opts, p.AudioBitrate)...)

	return appendHLSOutputArgs(args, opts.OutputDir, opts.UseFMP4, hlsTimeFor(opts), startNumberFor(opts), opts.VariantIndex)
}

func buildMultiVariantArgs(opts transcoding.HLSProcessOpts) []string {
	n := len(opts.Profiles)
	burnIn := opts.SubtitleTrackIndex >= 0 && IsImageSubtitle(opts.SubtitleCodec) && !opts.ForceSidecarSubs
	// Tonemap the single shared feed at the LARGEST rung width (folded into its
	// linearizing zscale) instead of the 4K source; the per-rung scaler below
	// then only downscales from there (top rung = no-op). Burn-in keeps a
	// full-resolution tonemap (overlay must happen at a fixed size, then scale).
	dstWidth := 0
	if !burnIn {
		for _, p := range opts.Profiles {
			if p.Width > dstWidth {
				dstWidth = p.Width
			}
		}
	}
	toneMap := resolveToneMap(opts, dstWidth)
	if hwUsable(opts, burnIn, toneMap) {
		return buildMultiVariantHWArgs(opts)
	}
	vcodec := resolveVideoEncoder(opts.VideoCodec)

	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = appendInputModifiers(args, opts.ReadRate)
	args = append(args, "-i", opts.FilePath)
	// Preserve source PTS in the output so an fMP4 fragment carries a
	// tfdt that matches the EXTINF position the Go-precomputed VOD
	// manifest gave hls.js for that segment. Without `-copyts`, a rafale
	// spawned at seg N>0 produces fragments whose tfdt restarts at 0;
	// the browser rejects them as "no decoder for the format" because
	// the init segment + fragment disagree on time origin.
	//
	// Deliberately NOT using `-avoid_negative_ts disabled` (the
	// single-variant builder uses it together with `-start_at_zero`).
	// fMP4 muxer requires non-negative timestamps, and we want
	// ffmpeg's default behaviour to shift if needed without zeroing
	// the absolute PTS.
	args = append(args, "-copyts")

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
				fmt.Sprintf("[0:v]%s,%s[vt]", deinterlaceFilterFor(opts), toneMap),
				fmt.Sprintf("[vt][0:s:%d]overlay[vb]", opts.SubtitleTrackIndex),
				fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		} else {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s[vd]", deinterlaceFilterFor(opts)),
				fmt.Sprintf("[vd][0:s:%d]overlay[vb]", opts.SubtitleTrackIndex),
				fmt.Sprintf("[vb]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		}
	} else {
		if toneMap != "" {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,%s[vt]", deinterlaceFilterFor(opts), toneMap),
				fmt.Sprintf("[vt]split=%d%s", n, strings.Join(splitOutputs, "")),
			)
		} else {
			filterParts = append(filterParts,
				fmt.Sprintf("[0:v]%s,split=%d%s", deinterlaceFilterFor(opts), n, strings.Join(splitOutputs, "")),
			)
		}
	}

	for i, p := range opts.Profiles {
		filterParts = append(filterParts, fmt.Sprintf("[v%d]%s[o%d]", i, scaleFilterFor(opts, p.Width), i))
	}
	args = append(args, "-filter_complex", strings.Join(filterParts, ";"))

	for i, p := range opts.Profiles {
		args = append(args,
			"-map", fmt.Sprintf("[o%d]", i),
			fmt.Sprintf("-c:v:%d", i), vcodec,
		)
		args = append(args, softwareProfileLevelArgs(vcodec, i)...) // h264-only; x265/av1 pick their own
		args = append(args,
			// Capped CRF per rung (see softwareCRF): quality-targeted, rung bitrate
			// as the hard ceiling, instead of a plain -b:v average.
			fmt.Sprintf("-crf:v:%d", i), softwareCRF,
			fmt.Sprintf("-maxrate:v:%d", i), fmt.Sprintf("%dk", p.VideoBitrate),
			fmt.Sprintf("-bufsize:v:%d", i), fmt.Sprintf("%dk", p.VideoBitrate*2),
			"-preset", "veryfast",
			"-pix_fmt", "yuv420p",
		)
		args = append(args, audioVariantArgs(opts, i, p.AudioBitrate)...)
	}
	// Force aligned keyframes at the segment cadence across every variant — both
	// to fix slow/irregular first-segment startup and to keep ABR-switchable
	// segment boundaries identical between renditions.
	args = append(args, hlsForceKeyframesArgs...)

	// -var_stream_map + HLS muxer tail (container choice, master playlist,
	// segment naming). Shared with the hardware multi-variant builder so both
	// paths produce byte-identical playlist/segment names for a given opts.
	return appendHLSMultiVariantTail(args, opts, n)
}

// resolveToneMap returns the HDR->SDR tonemap filter for this transcode, or "".
// It honours two signals: the explicit ToneMap decision baked into the opts by
// the orchestrated VOD path (which does not thread real Streams), and the
// stream-based detection used when real Streams ARE present (legacy/direct path).
// A client that renders HDR natively (HDRSupported) is never tonemapped.
// resolveToneMap returns the HDR->SDR tonemap filter, with the downscale to
// dstWidth folded into the tonemap's linearizing zscale so the heavy float
// pipeline runs at delivery resolution (see hdr.ToneMapOptions.DstWidth).
// dstWidth<=0 keeps a full-resolution tonemap (the caller scales separately,
// e.g. the subtitle burn-in path).
func resolveToneMap(opts transcoding.HLSProcessOpts, dstWidth int) string {
	if opts.HDRSupported {
		return ""
	}
	if opts.ToneMap {
		// Generic HDR10/HLG -> SDR (bt709) tonemap. The filter (zscale linearize
		// -> tonemap -> bt709) reads the input transfer/primaries from the decoded
		// frame's colour metadata, so it needs no per-stream params here.
		tmOpts := hdr.DefaultToneMapOptions()
		tmOpts.DstWidth = dstWidth
		tmOpts.UseTonemapx = tonemapxAvailable("ffmpeg")
		return hdr.BuildToneMapFilter(&hdr.HDRInfo{IsHDR: true}, tmOpts)
	}
	return detectToneMap(opts.HDRSupported, opts.Streams, dstWidth)
}

func detectToneMap(hdrSupported bool, streams []*library.MediaStream, dstWidth int) string {
	if hdrSupported {
		return ""
	}
	hdrStreams := make([]hdr.VideoStream, len(streams))
	for i, st := range streams {
		hdrStreams[i] = st
	}
	hdrInfo := hdr.DetectHDR(hdrStreams)
	if hdrInfo.IsHDR {
		tmOpts := hdr.DefaultToneMapOptions()
		tmOpts.DstWidth = dstWidth
		tmOpts.UseTonemapx = tonemapxAvailable("ffmpeg")
		return hdr.BuildToneMapFilter(hdrInfo, tmOpts)
	}
	return ""
}

// --- hardware-accelerated transcode path ---
//
// v1 handles the "simple" filtergraph: a zero-copy decode → GPU deinterlace
// (no-op on progressive, NVENC/VAAPI only) → GPU scale → GPU encode pipeline.
// HDR tonemapping and bitmap-subtitle burn-in still take the software path
// (their GPU filtergraphs are backend-specific and depend on optional ffmpeg
// filters that minimal/NAS builds may lack). The orchestrated VOD path
// currently always hits this simple case (Streams=nil, SubtitleCodec unset ⇒ no
// tonemap/burn-in), so v1 covers 100% of live VOD transcodes.
//
// The arg shapes here are validated end-to-end on real hardware (NVENC on an
// RTX 4070 Ti, VAAPI on a Ryzen iGPU): scale_cuda/scale_vaapi both accept a
// `-2` auto-height, split fans out GPU surfaces, and the *_nvenc/*_vaapi
// encoders consume them with no hwdownload round-trip.

// hwUsable reports whether the hardware encode path applies to this job. The
// wiring layer only sets HWAccel to a non-None value after confirming the flag
// is on and a detected device supports the codec; this gate additionally keeps
// the complex-filtergraph cases (tonemap, burn-in) on the proven software path.
func hwUsable(opts transcoding.HLSProcessOpts, burnIn bool, toneMap string) bool {
	if opts.HWAccel == "" || opts.HWAccel == transcoding.None || !opts.HWAccel.IsValid() || burnIn {
		return false
	}
	// A tone-map normally keeps the software path. The exception: the host probed
	// GPU tone-mapping AND it's wired into this request (opts.HWTonemap) AND the
	// backend has a HW tone-map filter — then the GPU folds HDR->SDR in-pipeline.
	if toneMap != "" {
		return opts.HWTonemap && hwTonemapFilter(opts.HWAccel) != ""
	}
	return true
}

// hwTonemapFilter returns the GPU HDR->SDR tone-map filter for the backend, or
// "" when no HW tone-mapper is wired. Inserted before the scaler so the GPU
// folds HDR (and 10-bit) in one pass. NVENC's HW tone-map path is libplacebo
// (Vulkan) which is not modelled here, so NVENC tone-maps in software.
func hwTonemapFilter(accel transcoding.HWAccelType) string {
	switch accel {
	case transcoding.VAAPI:
		return "tonemap_vaapi=format=nv12:matrix=bt709:primaries=bt709:transfer=bt709"
	case transcoding.QSV:
		return "vpp_qsv=tonemap=1:format=nv12"
	}
	return ""
}

// hwInitArgs returns the pre-`-i` flags that put decode + filtering on the GPU
// for a zero-copy transcode. NVENC selects the device through CUDA; VAAPI/QSV
// take an explicit DRM render node when one is provided (empty → ffmpeg
// default device).
func hwInitArgs(accel transcoding.HWAccelType, device string) []string {
	switch accel {
	case transcoding.NVENC:
		return []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}
	case transcoding.VAAPI:
		a := []string{"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi"}
		if device != "" {
			a = append(a, "-hwaccel_device", device)
		}
		return a
	case transcoding.QSV:
		a := []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
		if device != "" {
			a = append(a, "-qsv_device", device)
		}
		return a
	}
	return nil
}

// hwScaleFilter returns the GPU scaler for the backend. Width pins the output
// width; `-2` keeps the source aspect ratio rounded to an even height (which
// every HW encoder requires). format=nv12 forces 8-bit 4:2:0 output: 10-bit
// sources (HEVC Main10 / yuv420p10le, ubiquitous for 2160p) otherwise reach the
// 8-bit-only h264 HW encoders as 10-bit and fail with "10 bit encode not
// supported" → "device doesn't support required NVENC features", producing no
// segment. This mirrors the software path's `-pix_fmt yuv420p`. (No HDR tonemap
// here, matching the software path's current behaviour.)
func hwScaleFilter(accel transcoding.HWAccelType, width int) string {
	switch accel {
	case transcoding.NVENC:
		return fmt.Sprintf("scale_cuda=%d:-2:format=nv12", width)
	case transcoding.VAAPI:
		return fmt.Sprintf("scale_vaapi=%d:-2:format=nv12", width)
	case transcoding.QSV:
		return fmt.Sprintf("scale_qsv=%d:-2:format=nv12", width)
	}
	return fmt.Sprintf("scale=%d:-2", width)
}

// hwDeinterlaceFilter returns a GPU deinterlacer that, like the software
// deinterlaceFilter ("bwdif=deint=interlaced"), only touches frames the decoder
// flags as interlaced and passes progressive content through untouched — so it
// is always safe to prepend, never doubles the frame rate, and keeps the work
// on the GPU surfaces (no hwdownload round-trip). Returns "" for backends with
// no field-flag-respecting HW deinterlacer:
//   - NVENC/CUDA: yadif_cuda with deint=interlaced (mode 0 ⇒ one frame per frame)
//   - VAAPI: deinterlace_vaapi (auto=1 default ⇒ progressive passes through),
//     which covers AMD iGPUs and Intel iGPUs driven through VAAPI
//   - QSV: empty — vpp_qsv's deinterlace has no "only-if-flagged" mode and would
//     wrongly deinterlace progressive content; left to the software path / a
//     follow-up that gates on a probed interlace signal.
func hwDeinterlaceFilter(accel transcoding.HWAccelType) string {
	switch accel {
	case transcoding.NVENC:
		return "yadif_cuda=deint=interlaced"
	case transcoding.VAAPI:
		return "deinterlace_vaapi"
	}
	return ""
}

// hwEncoderQualityArgs returns the encoder-specific quality flags applied to
// the output stream currently being defined (no stream specifier — ffmpeg
// binds them to the next `-c:v`). The BASELINE (preset, plus NVENC's required
// -forced-idr) is always emitted and is byte-for-byte the pre-probe behaviour.
// The optional streaming tuning (b-frames, rc-lookahead, spatial/temporal AQ,
// b_ref_mode) is gated on `features` — the per-option set the calibration
// probe confirmed this host's encoder ACCEPTS (capability.HWFeatures forwarded
// via HLSProcessOpts.HWFeatures). An option absent from `features` is never
// passed, so an older NVENC/QSV/VAAPI chip that rejects it is never handed it,
// and the level-2 safety net catches anything the probe missed. Empty features
// ⇒ the proven conservative baseline. No `-pix_fmt yuv420p` — GPU surfaces.
func hwEncoderQualityArgs(accel transcoding.HWAccelType, features []string) []string {
	has := func(f string) bool {
		return slices.Contains(features, f)
	}
	switch accel {
	case transcoding.NVENC:
		// -forced-idr 1 is REQUIRED and unconditional: without it NVENC honours
		// -force_key_frames with plain (non-IDR) I-frames the HLS segmenter
		// won't cut on, so segment lengths drift off the precomputed VOD
		// cadence (validated on an RTX 4070 Ti). It is supported on every NVENC
		// generation, so it is not a probed option.
		args := []string{"-preset", "p4", "-forced-idr", "1"}
		if has("rc_lookahead") {
			args = append(args, "-rc-lookahead", "20")
		}
		if has("spatial_aq") {
			args = append(args, "-spatial-aq", "1")
		}
		if has("temporal_aq") {
			args = append(args, "-temporal-aq", "1")
		}
		if has("b_ref_mode") {
			args = append(args, "-b_ref_mode", "middle")
		}
		if has("bframes") {
			args = append(args, "-bf", "3")
		}
		return args
	case transcoding.QSV:
		// QSV baseline: veryfast keeps cold starts low on weak iGPUs. -forced_idr
		// is gated because an older pre-Skylake/Apollo-Lake QSV rejects unknown
		// options and would fail the whole encode — only emit it once probed.
		args := []string{"-preset", "veryfast"}
		if has("forced_idr") {
			args = append(args, "-forced_idr", "1")
		}
		return args
	case transcoding.VAAPI:
		// VAAPI rate control is the driver default; only add probed extras.
		var args []string
		if has("bframes") {
			args = append(args, "-bf", "3")
		}
		return args
	}
	return nil
}

// appendHLSMultiVariantTail writes the -var_stream_map + HLS muxer args shared
// by the software and hardware multi-variant builders, so both produce
// byte-identical playlist/segment naming for a given opts (a cache addressed by
// decision hash must not care whether SW or HW produced the segment).
func appendHLSMultiVariantTail(args []string, opts transcoding.HLSProcessOpts, n int) []string {
	var streamMap []string
	for i := range n {
		if hasAudio(opts) {
			streamMap = append(streamMap, fmt.Sprintf("v:%d,a:%d", i, i))
		} else {
			streamMap = append(streamMap, fmt.Sprintf("v:%d", i))
		}
	}
	args = append(args, "-var_stream_map", strings.Join(streamMap, " "))
	args = append(args,
		"-master_pl_name", "master.m3u8",
		"-f", "hls",
		"-hls_time", hlsSegmentSeconds,
		"-hls_list_size", "0",
		"-hls_playlist_type", "event",
		"-hls_flags", "temp_file",
		"-start_number", fmt.Sprintf("%d", startNumberFor(opts)),
	)
	if opts.UseFMP4 {
		args = append(args,
			"-hls_segment_type", "fmp4",
			"-hls_fmp4_init_filename", "stream_%v_init.mp4",
			"-hls_segment_filename", filepath.Join(opts.OutputDir, "stream_%v_%03d.m4s"),
		)
	} else {
		args = append(args,
			"-hls_segment_filename", filepath.Join(opts.OutputDir, "stream_%v_%03d.ts"),
		)
	}
	args = append(args, filepath.Join(opts.OutputDir, "stream_%v.m3u8"))
	return args
}

// buildSingleTranscodeHWArgs is the GPU single-rung equivalent of
// buildSingleTranscodeArgs. Mirrors that builder's container choice (.ts) so a
// SW↔HW fallback never changes on-disk naming for the same job.
func buildSingleTranscodeHWArgs(opts transcoding.HLSProcessOpts) []string {
	p := opts.Profiles[len(opts.Profiles)-1]

	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = append(args, hwInitArgs(opts.HWAccel, opts.HWDevice)...)
	args = appendInputModifiers(args, opts.ReadRate)
	args = append(args, "-i", opts.FilePath)

	vf := hwScaleFilter(opts.HWAccel, p.Width)
	if opts.HWTonemap {
		if tm := hwTonemapFilter(opts.HWAccel); tm != "" {
			vf = tm + "," + vf // GPU HDR->SDR fold before the scaler
		}
	}
	if d := hwDeinterlaceFilter(opts.HWAccel); d != "" {
		vf = d + "," + vf
	}
	args = append(args, "-map", "0:v:0", "-vf", vf)
	args = append(args, "-c:v", videoEncoderFor(opts.VideoCodec, opts.HWAccel))
	args = append(args, hwEncoderQualityArgs(opts.HWAccel, opts.HWFeatures)...)
	args = append(args, "-b:v", fmt.Sprintf("%dk", p.VideoBitrate))
	args = append(args, hlsForceKeyframesArgs...)
	args = append(args, audioOutputArgsSingle(opts, p.AudioBitrate)...)

	return appendHLSOutputArgs(args, opts.OutputDir, opts.UseFMP4, hlsTimeFor(opts), startNumberFor(opts), opts.VariantIndex)
}

// buildMultiVariantHWArgs is the GPU equivalent of buildMultiVariantArgs: a
// single decode fed into a GPU split, one GPU scaler per rung, one HW encoder
// per rung — zero host-memory round-trip. Audio stays software-encoded.
func buildMultiVariantHWArgs(opts transcoding.HLSProcessOpts) []string {
	n := len(opts.Profiles)
	vcodec := videoEncoderFor(opts.VideoCodec, opts.HWAccel)

	var args []string
	if opts.StartSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", opts.StartSec))
	}
	args = append(args, hwInitArgs(opts.HWAccel, opts.HWDevice)...)
	args = appendInputModifiers(args, opts.ReadRate)
	args = append(args, "-i", opts.FilePath)
	// Same source-PTS preservation as the software multi-variant builder: a
	// rafale spawned at seg N>0 must emit fMP4 fragments whose tfdt matches the
	// VOD manifest's EXTINF, or the browser rejects them.
	args = append(args, "-copyts")

	splitOutputs := make([]string, n)
	for i := range n {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}
	// Deinterlace once on the source surface before the GPU split, so every rung
	// scales from progressive frames (no-op on progressive sources).
	src := "[0:v]"
	if d := hwDeinterlaceFilter(opts.HWAccel); d != "" {
		src += d + ","
	}
	// Tone-map once on the source surface before the GPU split (not per-rung).
	if opts.HWTonemap {
		if tm := hwTonemapFilter(opts.HWAccel); tm != "" {
			src += tm + ","
		}
	}
	filterParts := []string{fmt.Sprintf("%ssplit=%d%s", src, n, strings.Join(splitOutputs, ""))}
	for i, p := range opts.Profiles {
		filterParts = append(filterParts, fmt.Sprintf("[v%d]%s[o%d]", i, hwScaleFilter(opts.HWAccel, p.Width), i))
	}
	args = append(args, "-filter_complex", strings.Join(filterParts, ";"))

	for i, p := range opts.Profiles {
		args = append(args, "-map", fmt.Sprintf("[o%d]", i), fmt.Sprintf("-c:v:%d", i), vcodec)
		args = append(args, hwEncoderQualityArgs(opts.HWAccel, opts.HWFeatures)...)
		args = append(args, fmt.Sprintf("-b:v:%d", i), fmt.Sprintf("%dk", p.VideoBitrate))
		args = append(args, audioVariantArgs(opts, i, p.AudioBitrate)...)
	}
	args = append(args, hlsForceKeyframesArgs...)

	return appendHLSMultiVariantTail(args, opts, n)
}

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

	// HLS muxer — event-mode, fMP4 container, temp_file for atomic renames.
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
		stderr:       stderr,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("ffmpeg audio-only start: %w", err)
	}

	go h.run(processCtx)
	return h, nil
}

// audioHandle controls a running audio-only HLS ffmpeg process. It satisfies
// transcoding.HLSProcessHandle so the SegmentOrchestrator can use it
// polymorphically alongside the video hlsHandle.
type audioHandle struct {
	outputDir    string
	playlistName string
	mu           sync.Mutex
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	ready        chan struct{}
	done         chan struct{}
	err          error
	exitErr      error // raw ffmpeg exit error, for classification (guarded by mu)
	stderr       *ringBuffer
	paused       bool
}

func (h *audioHandle) setExitErr(err error) {
	h.mu.Lock()
	h.exitErr = err
	h.mu.Unlock()
}

func (h *audioHandle) run(ctx context.Context) {
	defer close(h.done)

	playlistPath := filepath.Join(h.outputDir, h.playlistName)
	if err := waitForFile(ctx, playlistPath, 30*time.Second, 100*time.Millisecond); err != nil {
		h.err = fmt.Errorf("waiting for %s: %w", h.playlistName, err)
		h.cancel()
		we := h.cmd.Wait()
		h.setExitErr(we)
		if we != nil {
			slog.Debug("ffmpeg audio wait after cancel", slog.Any("err", we))
		}
		close(h.ready)
		return
	}

	close(h.ready)

	we := h.cmd.Wait()
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
				slog.Warn("ffmpeg audio kill after timeout failed", slog.Any("err", err))
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
