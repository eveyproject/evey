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
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

var _ transcoding.HLSProcess = (*HLSTranscoder)(nil)

type HLSTranscoder struct{}

func NewHLSTranscoder() *HLSTranscoder {
	return &HLSTranscoder{}
}

func (t *HLSTranscoder) Start(ctx context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	// The one chokepoint every producer reaches before a process is forked.
	// Refusing here names the caller's mistake; letting it through spends a
	// fork and surfaces as an ffmpeg exit code that reads like a broken file.
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	// Probe (cached) the source's audio start delay here - once per session,
	// before any per-variant fan-out - so BuildFFmpegArgs stays pure.
	opts.AudioStartDelaySec = audioStartDelayFor(ctx, opts)
	// Per-variant HW: one single-output ffmpeg per ABR rung behind a composite
	// handle, instead of one multi-output process. Matches how mature servers
	// drive HW - the multi-encode-in-one-process graph crashes fragile VA
	// drivers (and the WSL d3d12 shim) and a single rung's failure shouldn't
	// take the others down. Software keeps the single-process multi-variant
	// graph (decode once, encode N - efficient and crash-free on libx264).
	if opts.PerVariantHW && opts.HWAccel != "" && opts.HWAccel != transcoding.None && len(opts.Profiles) > 1 {
		return startPerVariant(ctx, opts)
	}
	h, err := startOne(ctx, opts, false)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// needsFMP4Rebase: fMP4 sessions whose segments come out session-relative
// (0-based) and must be shifted to the absolute timeline before publishing,
// because movenc always writes dts − first_dts and no CLI flag yields absolute
// time (see fmp4_rebase.go).
//
//   - copy + fMP4: ALWAYS 0-based → always rebase. Cold copy (StartNumber 0)
//     is absolute by construction (deltaMs 0, parse-only) but still routes
//     through the pump so the timeline TRIPWIRE (verifySegmentTimeline)
//     covers it - the historical numbering-drift bug lived on the cold path.
//   - transcode SEEK: pre-input `-ss` resets the output PTS to 0, so the seek
//     segment's tfdt is 0 instead of StartSec → the browser can't place it on
//     the VOD timeline and seeking stalls in permanent buffering. These need
//     the same shift. A seek is recognised by EITHER StartNumber or StartSec:
//     the segment orchestrator only ever filled StartSec (startNumberFor
//     derives -start_number from it), so keying on StartNumber alone left
//     every transcode seek un-rebased - 0-based fragments under absolute
//     names in the shared cache. Cold transcode (start 0) is absolute by
//     construction and stays on the direct-write path.
func needsFMP4Rebase(o transcoding.HLSProcessOpts) bool {
	if !o.UseFMP4 {
		return false
	}
	return o.VideoDecision == "copy" || o.StartNumber > 0 || o.StartSec > 0
}

// startOne spawns a single ffmpeg process for opts and returns its handle.
// noMaster suppresses the synthesised one-rung master.m3u8 (set for the
// sub-processes of a per-variant composite).
func startOne(ctx context.Context, opts transcoding.HLSProcessOpts, noMaster bool) (*hlsHandle, error) {
	// Copy-seek sessions write into a private subdir; runRebasePump shifts
	// their fMP4 timestamps to the plan's absolute StartSec and atomically
	// renames into the cache dir - the rename is the publish point
	// SegmentCache.IsReady observes, so readers never see a pre-rebase file.
	pubDir := opts.OutputDir
	rebase := needsFMP4Rebase(opts)
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
	// this the audio-only-failure cases are invisible - ffmpeg writes a
	// fatal error on stderr and we'd otherwise just see the consumer's
	// generic 5xx with no clue why.
	stderr := newRingBuffer(8 * 1024)
	cmd.Stderr = stderr

	h := &hlsHandle{
		opts:       opts,
		cmd:        cmd,
		cancel:     cancel,
		ready:      make(chan struct{}),
		done:       make(chan struct{}),
		procExited: make(chan struct{}),
		stderr:     stderr,
		noMaster:   noMaster,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		if rebase {
			_ = os.RemoveAll(opts.OutputDir)
		}
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}
	// Single owner of cmd.Wait: everyone else reads awaitProcExit().
	go func() {
		h.procWaitErr = cmd.Wait()
		close(h.procExited)
	}()

	go h.run(processCtx)
	if rebase {
		deltaMs := int64(opts.StartSec*1000 + 0.5)
		go runRebasePump(h.done, processCtx.Done(), h.procExited, opts.OutputDir, pubDir, deltaMs, "stream", opts.VariantIndex, opts.ExpectedSegmentStartMs, opts.TimelineDriftKind)
	}
	return h, nil
}

// startPerVariant spawns one single-output ffmpeg per profile (each writing its
// own stream_<i>_* rung) and returns a compositeHandle that fans Stop/Pause/
// Resume to all and aggregates Done/Failure. The composite ctx cancel kills
// every rung at once.
func startPerVariant(ctx context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	procCtx, cancel := context.WithCancel(ctx)
	c := &compositeHandle{ctx: procCtx, cancel: cancel, done: make(chan struct{})}
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

// IsImageSubtitle re-exports the domain's single source of truth (the adapter
// may import the domain - what it must not import is the application layer,
// which is why this used to be a hand-synced duplicate).
func IsImageSubtitle(codec string) bool {
	return transcoding.IsImageSubtitle(codec)
}

// hasAudio reports whether the source has a selectable audio track. The wiring
// layer signals "no audio" with AudioTrackIndex < 0 (rare: a silent/video-only
// media). When false the builders emit a video-only ladder - no -map 0:a, no
// audio encode args, and the var_stream_map drops the audio rendition - so
// ffmpeg doesn't abort with "Invalid argument" on a missing 0:a:N stream.
func hasAudio(opts transcoding.HLSProcessOpts) bool {
	return opts.AudioTrackIndex >= 0
}

// audioDelayCancelMinSec is the floor below which a probed source audio start
// delay is ignored. Legitimate small offsets (AAC priming ~21-40ms, muxer
// interleave jitter) live well under it; the mux-defect class this fix targets
// measures hundreds of ms. Below the floor the args stay byte-identical.
const audioDelayCancelMinSec = 0.1

// audioNetShiftSec is the single net input shift the second audio input
// applies: the source-defect cancellation (shift audio EARLIER by the probed
// start delay) minus the user's resync request (positive = audio LATER). The
// two compose into one -itsoffset value: -net.
func audioNetShiftSec(opts transcoding.HLSProcessOpts) float64 {
	return opts.AudioStartDelaySec - opts.AudioUserOffsetSec
}

// audioDelayCancelActive reports whether this session feeds audio from a
// second, -itsoffset-shifted input. Only TRANSCODED audio is eligible: copied
// audio packets cannot be trimmed at the seek edge, and the early shifted
// packets were measured (jellyfin-ffmpeg 7.1) to hijack the make_zero
// zero-point of copy-on-seek sessions - the video landed +0.87s off plan and
// the copy timeline tripwire fired. Two triggers:
//   - the probed SOURCE start delay exceeds the noise floor (auto-cancel);
//   - the USER requested a resync (honored exactly, no floor - they clicked
//     a 100 ms step on purpose), unless it nets out to zero against the
//     source delay.
func audioDelayCancelActive(opts transcoding.HLSProcessOpts) bool {
	if !hasAudio(opts) || opts.AudioDecision != "transcode" {
		return false
	}
	if opts.AudioUserOffsetSec != 0 {
		return audioNetShiftSec(opts) != 0
	}
	return opts.AudioStartDelaySec > audioDelayCancelMinSec
}

// audioInput is the ffmpeg input index the audio maps read from: 0 (the main
// input) normally, 1 (the delay-cancelling second input) when active.
func audioInput(opts transcoding.HLSProcessOpts) int {
	if audioDelayCancelActive(opts) {
		return 1
	}
	return 0
}

// appendAudioDelayInput appends the delay-cancelling audio input right after
// the caller's main input: `[-ss <ssSec>] [pacing] -itsoffset -<delay> -i FILE`.
// ssSec > 0 replicates the caller's seek verbatim - ffmpeg compares the seek
// target against POST-itsoffset timestamps, so the replica needs no +delay
// adjustment (measured: the accurate-seek trim lands the shifted audio exactly
// on the scene the video starts on). pacing mirrors input 0's pacing flags; an
// unpaced audio input next to a paced video input would re-create the very
// interleave skew the pacing bounds. No-op when the cancel is inactive.
func appendAudioDelayInput(args []string, opts transcoding.HLSProcessOpts, ssSec float64, pacing func([]string) []string) []string {
	if !audioDelayCancelActive(opts) {
		return args
	}
	if ssSec > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", ssSec))
	}
	if pacing != nil {
		args = pacing(args)
	}
	return append(args, "-itsoffset", fmt.Sprintf("%.3f", -audioNetShiftSec(opts)), "-i", opts.FilePath)
}

// audioOutputArgsSingle returns the audio map + single-stream encode/copy args
// for the copy and single-variant transcode builders, or nil when the source
// has no audio. Honours the copy-vs-transcode decision and optional loudnorm.
func audioOutputArgsSingle(opts transcoding.HLSProcessOpts, bitrateKbps int64) []string {
	if !hasAudio(opts) {
		return nil
	}
	a := []string{"-map", fmt.Sprintf("%d:a:%d", audioInput(opts), opts.AudioTrackIndex)}
	if opts.AudioDecision == "transcode" {
		a = append(a,
			"-c:a", resolveAudioEncoder(opts.AudioCodec),
			"-ac", resolveAudioChannels(opts.MaxAudioChannels),
			"-b:a", fmt.Sprintf("%dk", scaledAudioBitrateKbps(bitrateKbps, opts.MaxAudioChannels)),
		)
		a = append(a, transcoding.AudioDownmixArgs(opts.MaxAudioChannels)...)
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
		"-map", fmt.Sprintf("%d:a:%d", audioInput(opts), opts.AudioTrackIndex),
		fmt.Sprintf("-c:a:%d", i), resolveAudioEncoder(opts.AudioCodec),
		"-ac", resolveAudioChannels(opts.MaxAudioChannels),
		fmt.Sprintf("-b:a:%d", i), fmt.Sprintf("%dk", scaledAudioBitrateKbps(bitrateKbps, opts.MaxAudioChannels)),
	}
	a = append(a, transcoding.AudioDownmixArgs(opts.MaxAudioChannels)...)
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
// the OutputDir at session end - no manual segment deletion needed. Worst case
// is bitrate x media_duration per session (~5 GB for a 2h 1080p multivariant).

// hlsSegmentSeconds is the target HLS segment length (the -hls_time cadence for
// every video AND audio-rendition session). It MUST stay in sync with the
// -force_key_frames cadence (hlsForceKeyframesArgs) so ffmpeg emits a keyframe
// exactly at each segment boundary, and with apptranscoding.ManifestSegmentSeconds
// so the manifests advertise exactly as many segments as the encoder cuts.
// TestSegmentGridConstantsInSync enforces the latter across the two packages.
const hlsSegmentSeconds = "3"

// deinterlaceFilter deinterlaces only frames the decoder flags as interlaced
// (deint=interlaced), at the SOURCE frame rate (mode=send_frame). bwdif's
// DEFAULT mode is send_field, which emits one output frame per FIELD - doubling
// the frame rate (e.g. 24→48 fps) even on progressive content, because the rate
// doubling is independent of deint=interlaced (that flag only gates the
// deinterlace *algorithm*, not the field-to-frame expansion). Applied unconditionally
// at the head of the filter chain, send_field silently ran scale + tonemap + the
// x264 encode at 2× the frames: a 4K HEVC10 HDR transcode measured speed=0.90×
// realtime on an old iGPU box (frame=10798 over 224s = 48 fps, dup≈half) - sub-realtime,
// so playback buffered indefinitely with no lower rung to fall back to.
// send_frame keeps the combing fix for genuinely interlaced sources (old TV /
// repacks) at their native rate and is a true 1:1 no-op on progressive content.
const deinterlaceFilter = "bwdif=mode=send_frame:deint=interlaced"

// deinterlaceFilterFor returns the software deinterlacer for these opts. The
// default flag-gated bwdif (deint=interlaced) is a no-op on progressive
// content; when the idet content detection flagged the source interlaced
// despite its container saying otherwise (DeinterlaceAll), it switches to
// deint=all so every frame is deinterlaced - the only way to clear the
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
// manifest advertises (still ABR-safe - a peak can't blow past what the player
// budgeted). Lowered from the 23 x264 default to 20: the software path encodes
// at the realtime veryfast preset, so a tighter CRF buys back sharpness for ~the
// same encode cost, and the benched ceiling (also veryfast) is CRF-independent.
// HW encoders ignore CRF (rate-control differs), so this is the software path only.
const softwareCRF = "20"

// maxrateHeadroomPct sizes the -maxrate ceiling above the rung's advertised
// bitrate. The two are different jobs: the rung bitrate is the manifest's
// BANDWIDTH (what ABR budgets), the ceiling should only clip outliers. Pinned
// together, CRF 20 was strangled on re-encodes of already-lossy sources (old
// XviD .avi), whose grain is expensive to reproduce. Raising the rung bitrate
// instead would push constrained viewers down a whole rung.
//
// Not part of the DecisionFingerprint (like softwareCRF): existing segments
// keep the ceiling they were encoded with until the cache is purged.
const maxrateHeadroomPct = 160

// cappedCRFRateArgs returns the -maxrate/-bufsize pair for a rung advertised
// at kbps. bufsize keeps a 2x-of-ceiling VBV window.
func cappedCRFRateArgs(kbps int64) (maxrate, bufsize string) {
	ceiling := kbps * maxrateHeadroomPct / 100
	return fmt.Sprintf("%dk", ceiling), fmt.Sprintf("%dk", ceiling*2)
}

// svtAV1Preset is the libsvtav1 numeric speed/quality preset (0-13, higher =
// faster). 10 leans fast for the weak-NAS software-AV1 path; tune here.
const svtAV1Preset = "10"

// hlsForceKeyframesArgs forces a keyframe exactly every segment. Without it,
// x264's sparse default GOP (~10s) made the HLS muxer wait many seconds for a
// keyframe before it could close the FIRST segment - the dominant cause of the
// multi-second startup delay - and produced irregular segment lengths. The
// expr form is fps-independent. Negligible bitrate cost at a 3s cadence.
var hlsForceKeyframesArgs = []string{"-force_key_frames", "expr:gte(t,n_forced*" + hlsSegmentSeconds + ")"}

// appendInputModifiers is the hook for pre-`-i` flags. It emits `-readrate N`
// (input pacing: read the source at N× real-time) ONLY when readRate > 0.
//
// Pacing is per-producer, not global. The multi-rafale HW orchestrator leaves
// readRate=0 and burns CPU on segments 0-N until its adaptive throttle (SIGSTOP
// when the client is far ahead, SIGCONT on CMCD bs=1) kicks in - removing the
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

// transcodeColdStartBurstSec is how many opening seconds a PACED transcode
// reads at full speed before the -readrate pace kicks in (ffmpeg
// -readrate_initial_burst, ≥6.1 - present in the bundled jellyfin-ffmpeg7 and
// already relied on by the copy rung).
//
// 60 s (20 segments), raised from the original 15 s on measurement rather than
// taste. Head-to-head against an UNPACED server on the same host (2026-08-08,
// staggered arrivals, 150 s window): both served the same number of clients -
// 8/8 on a 1080p H.264 source, 2/2 then 1/3 on 4K HEVC - so pacing costs no
// viewers at all. What differed was the RESERVE: the unpaced clients banked
// 472 s of lead where the paced ones held 230 s, and a session that never gets
// more than ~1.5x ahead has nothing to spend when a spike arrives, so it
// starves immediately where a banked one rides it out.
//
// The burst is what buys that reserve back without giving up the steady-state
// smoothing, and it stays bounded on both ends: a fixed span of content per
// session (a handful of wall-clock seconds on any host that transcodes faster
// than realtime), and the VideoSessionLimiter still caps how many encodes may
// exist at once from the host's own calibration.
const transcodeColdStartBurstSec = "60"

// appendTranscodeInputModifiers emits the input pacing for a TRANSCODE session:
// -readrate N plus the cold-start burst, for EVERY codec.
//
// The burst was AV1-only at first, blamed on SVT-AV1's encoder latency. That
// was the wrong variable: what makes the opening segments late is the INPUT
// pace, and it is codec-blind. At 1.5x, the third 3 s segment cannot exist
// before 9/1.5 = 6 s of reading plus ffmpeg's startup, no matter how fast the
// encoder downstream is. Measured in prod 2026-07-25 on a 4K HEVC HDR -> h264
// 1080p transcode: the player abandoned a segment at 7.75 s that was
// arithmetically unreachable, on a host that was NOT saturated. Worse, pacing
// from the first frame grows the viewer's buffer at only 0.5x real time (1.5
// produced - 1.0 consumed), so nothing absorbs an early hiccup - the reason
// this path felt worse than an unpaced server on identical hardware.
//
// The original reservation (a startup CPU burst is undesirable when several
// streams share a weak NAS) is bounded on both ends: the burst lasts a fixed
// handful of seconds, and the VideoSessionLimiter already caps how many
// encodes may exist at once from the host's own calibration. Steady state is
// unchanged - still readRate, with the adaptive throttle pausing a fat buffer.
//
// readRate == 0 (the HW orchestrator, which self-throttles with SIGSTOP
// instead) emits neither flag: the burst rides ON the pace, and an unpaced
// session must stay byte-identical.
func appendTranscodeInputModifiers(args []string, readRate float64) []string {
	args = appendInputModifiers(args, readRate)
	if readRate > 0 {
		args = append(args, "-readrate_initial_burst", transcodeColdStartBurstSec)
	}
	return args
}

// appendHLSOutputArgs writes the HLS muxer arguments shared by every
// builder. list_size 0 + playlist_type event keep all segments available for
// the full session duration; useFMP4 picks the segment container.
// `temp_file` makes ffmpeg write each segment to `<name>.tmp` first, then
// rename atomically on close - so a concurrent reader never sees a partial
// segment (audit indies: Stash pattern, free with this flag on Linux/macOS).
//
// startNumber controls ffmpeg's HLS segment counter (and so the segment
// filenames stream_N_NNN.{ts|m4s}). When the rafale starts mid-media via
// `-ss`, callers must compute startNumber = floor(StartSec/hlsSegmentSeconds)
// so the produced filenames match the absolute manifest indices the VOD
// pipeline cache addresses by. Pass 0 for legacy / non-rafale callers.
// hlsTimeFor returns the -hls_time value. The COPY rung always runs "0" - split
// at EVERY keyframe - because that is the only muxer split rule that is
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
// The `streams` arg is no longer consulted - historically we forced
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
// max_interleave_delta force-flushes - measured in prod as audio landing up to
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
		// SHARED per variant (stream_N_init.mp4) - every session, seek or not,
		// overwrites it and the player decodes ALL segments against whichever init
		// it loaded. -copyts preserved the absolute source PTS, which fMP4 records
		// as an EDIT LIST in the init (elst media_time = StartSec). A seek session
		// thus wrote a NON-canonical init that presentation-shifted EVERY segment -
		// including the t=0 ones from the cold session - by +StartSec. Proven in a
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
		// rebase) is set to that landing - but ffmpeg's default accurate-seek
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
		args = append(args, "-i", opts.FilePath)
		// Delay-cancelled audio input keeps ffmpeg's DEFAULT accurate seek (no
		// -noaccurate_seek replica): the decode trim is what drops the shifted
		// audio content sitting before the seek target, so the session's audio
		// starts at the target instead of hijacking the make_zero zero-point.
		args = appendAudioDelayInput(args, opts, target, func(a []string) []string {
			a = appendInputModifiers(a, opts.ReadRate)
			return appendCopyPacingBurst(a, opts.ReadRate)
		})
		args = append(args, "-map", "0:v:0")
		if hasAudio(opts) {
			args = append(args, "-map", fmt.Sprintf("%d:a:%d", audioInput(opts), opts.AudioTrackIndex))
		}
		args = append(args, "-c:v", "copy", "-avoid_negative_ts", "make_zero")
		args = append(args, copyVideoTagArgs(opts)...)
	case opts.StartSec > 0:
		// the field-proven recipe for HLS video transcode (verified
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
		args = appendAudioDelayInput(args, opts, opts.StartSec, func(a []string) []string {
			return appendInputModifiers(a, opts.ReadRate)
		})
		args = append(args, "-map", "0:v:0")
		if hasAudio(opts) {
			args = append(args, "-map", fmt.Sprintf("%d:a:%d", audioInput(opts), opts.AudioTrackIndex))
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
		args = appendAudioDelayInput(args, opts, 0, func(a []string) []string {
			a = appendInputModifiers(a, opts.ReadRate)
			return appendCopyPacingBurst(a, opts.ReadRate)
		})
		args = append(args, "-map", "0:v:0")
		if hasAudio(opts) {
			args = append(args, "-map", fmt.Sprintf("%d:a:%d", audioInput(opts), opts.AudioTrackIndex))
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
			args = append(args, transcoding.AudioDownmixArgs(opts.MaxAudioChannels)...)
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
// when the master's CODECS attribute says hvc1 - the attribute and the actual
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
// 153 s with 3 s segments lands inside seg 51 - start_number must be 51.
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

// cpuBudgetThreadsArgs bounds a SOFTWARE encoder's thread count to what this
// process may actually use: the operator's explicit
// EVEY_TRANSCODING_SOFTWARE_THREADS override when set (softwareTranscodeThreads,
// unchanged), else sysinfo.CPUBudget() - the cgroup CPU quota when the container
// runs under one, else NumCPU. Without this, libx264/libx265/libsvtav1 leave
// -threads unset and grab every HOST core even when a cgroup quota caps this
// process to a fraction of them, starving every other job (scanning, metadata,
// serving the UI) on a weak NAS. Hardware encoders (nvenc/vaapi/qsv) never reach
// this - they are routed to their own HW arg builders before either call site
// below, and -threads has a different (non-CPU-bounding) meaning for them.
func cpuBudgetThreadsArgs(streamSpec string) []string {
	if args := softwareThreadsArgs(streamSpec); args != nil {
		return args // explicit operator override (EVEY_TRANSCODING_SOFTWARE_THREADS)
	}
	return softwareThreadsArgsN(sysinfo.CPUBudget(), streamSpec)
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
	args = appendTranscodeInputModifiers(args, opts.ReadRate)
	args = append(args, "-i", opts.FilePath)
	args = appendAudioDelayInput(args, opts, opts.StartSec, func(a []string) []string {
		return appendTranscodeInputModifiers(a, opts.ReadRate)
	})

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
			// zscale), so no trailing scale - that 4× cut in tonemapped pixels is
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
	args = append(args, cpuBudgetThreadsArgs("")...)          // bound to the CPU budget (cgroup quota or NumCPU); operator override wins if set
	args = append(args, softwareProfileLevelArgs(enc, -1)...) // h264-only; x265/av1 pick their own
	args = append(args, softwareX264TuningArgs(enc, -1)...)   // motion-search tuning: +24% on encode-bound sources
	args = append(args,
		// Capped CRF (see softwareCRF): quality-targeted with the rung bitrate as a
		// hard ceiling, instead of a plain -b:v average.
		"-crf", softwareCRF,
	)
	maxrate, bufsize := cappedCRFRateArgs(p.VideoBitrate)
	args = append(args, "-maxrate", maxrate, "-bufsize", bufsize)
	args = append(args, svtAV1PresetArgs(enc)...)
	args = append(args, "-pix_fmt", "yuv420p")
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
	args = appendTranscodeInputModifiers(args, opts.ReadRate)
	args = append(args, "-i", opts.FilePath)
	args = appendAudioDelayInput(args, opts, opts.StartSec, func(a []string) []string {
		return appendTranscodeInputModifiers(a, opts.ReadRate)
	})
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
		args = append(args, cpuBudgetThreadsArgs(fmt.Sprintf(":v:%d", i))...) // bound to the CPU budget per rung; operator override wins if set
		args = append(args, softwareProfileLevelArgs(vcodec, i)...)           // h264-only; x265/av1 pick their own
		args = append(args, softwareX264TuningArgs(vcodec, i)...)
		args = append(args,
			// Capped CRF per rung (see softwareCRF): quality-targeted, rung bitrate
			// as the hard ceiling, instead of a plain -b:v average.
			fmt.Sprintf("-crf:v:%d", i), softwareCRF,
		)
		maxrate, bufsize := cappedCRFRateArgs(p.VideoBitrate)
		args = append(args,
			fmt.Sprintf("-maxrate:v:%d", i), maxrate,
			fmt.Sprintf("-bufsize:v:%d", i), bufsize,
		)
		args = append(args, svtAV1PresetArgs(vcodec)...)
		args = append(args, "-pix_fmt", "yuv420p")
		args = append(args, audioVariantArgs(opts, i, p.AudioBitrate)...)
	}
	// Force aligned keyframes at the segment cadence across every variant - both
	// to fix slow/irregular first-segment startup and to keep ABR-switchable
	// segment boundaries identical between renditions.
	args = append(args, hlsForceKeyframesArgs...)

	// -var_stream_map + HLS muxer tail (container choice, master playlist,
	// segment naming). Shared with the hardware multi-variant builder so both
	// paths produce byte-identical playlist/segment names for a given opts.
	return appendHLSMultiVariantTail(args, opts, n)
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
