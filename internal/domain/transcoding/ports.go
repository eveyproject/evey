package transcoding

import (
	"context"
	"errors"
	"fmt"

	// library.MediaStream is used in HLSProcessOpts. Duplicating it here would
	// be worse: the struct has 20+ fields, is referenced in 24 files, and
	// transcoding inherently depends on stream metadata (codec, resolution, HDR).
	"github.com/evey-project/evey/internal/domain/library"
)

type Transcoder interface {
	Start(ctx context.Context, job *TranscodeJob) (string, error)
	Stop(ctx context.Context, jobID string) error
	Progress(ctx context.Context, jobID string) (*TranscodeProgress, error)
	// Pause suspends the job's encoder (SIGSTOP): it yields the CPU without
	// losing progress. Resume (SIGCONT) continues it. Used to let live playback
	// pre-empt background transcode jobs so watching never competes for cores.
	Pause(ctx context.Context, jobID string) error
	Resume(ctx context.Context, jobID string) error
}

type HWDetector interface {
	DetectDevices(ctx context.Context) ([]HWDevice, error)
	SupportsCodec(ctx context.Context, deviceType, codec string) (bool, error)
}

type TranscodeJobRepository interface {
	Create(ctx context.Context, job *TranscodeJob) error
	GetByID(ctx context.Context, id string) (*TranscodeJob, error)
	Update(ctx context.Context, job *TranscodeJob) error
	ListActive(ctx context.Context) ([]*TranscodeJob, error)
	ListByMediaItem(ctx context.Context, mediaItemID string) ([]*TranscodeJob, error)
}

type TranscodeProfileRepository interface {
	GetByID(ctx context.Context, id string) (*TranscodeProfile, error)
	List(ctx context.Context) ([]*TranscodeProfile, error)
	Create(ctx context.Context, profile *TranscodeProfile) error
	Update(ctx context.Context, profile *TranscodeProfile) error
	Delete(ctx context.Context, id string) error
}

type CacheStore interface {
	GetCacheInfo(ctx context.Context) (*CacheInfo, error)
	ClearCache(ctx context.Context) error
	DeleteFile(ctx context.Context, filename string) error
}

// HLSProcess spawns and manages an ffmpeg HLS transcode process.
type HLSProcess interface {
	Start(ctx context.Context, opts HLSProcessOpts) (HLSProcessHandle, error)
}

// CopySeekNudgeSec is the offset added to a copy session's pre-input -ss so it
// lands ON the intended keyframe rather than snapping to the previous one.
// StartSec derives from the ms-rounded keyframe index, so it can sit a hair
// BELOW the true keyframe time; -ss seeks to the point at-or-before the
// timestamp, so an exact-or-low target lands one keyframe early (proven in a
// jellyfin-ffmpeg7 harness: 1 ms below → previous keyframe; +50 ms → correct,
// robust to ~1-frame index error; other players use up to +0.5 s for the same quirk).
// Shared between the ffmpeg arg builder (-ss emission) and the copy producer's
// seek-landing probe so both reason about the SAME effective seek target.
const CopySeekNudgeSec = 0.05

// AudioProcess spawns and manages an ffmpeg audio-only HLS rendition process.
// It produces audio_T_init.mp4 + audio_T_NNN.m4s segments alongside the video
// pipeline so the SegmentOrchestrator can serve them as an EXT-X-MEDIA rendition.
type AudioProcess interface {
	StartAudioOnly(ctx context.Context, opts AudioProcessOpts) (HLSProcessHandle, error)
}

// AudioProcessOpts configures a single audio-only HLS rendition encode.
// T in filenames refers to SourceTrackIdx (zero-indexed audio stream in source).
type AudioProcessOpts struct {
	FilePath         string  // source media absolute path
	OutputDir        string  // destination for audio_T_NNN.m4s + audio_T_init.mp4
	SourceTrackIdx   int     // 0-indexed audio stream in source
	AudioCodec       string  // "aac", "ac3", "eac3", "opus" → resolveAudioEncoder
	AudioBitrate     int     // kbps; 0 = encoder default
	MaxAudioChannels int     // for resolveAudioChannels; 0 = passthrough (→ "2")
	StartSec         float64 // for mid-stream rafale; 0 for cold start
	StartNumber      int     // ffmpeg -start_number; floor(StartSec/3) for rafale
	AudioNormalize   bool    // insert loudnorm filter
	// AudioEncoder names the concrete ffmpeg AAC implementation this rendition
	// was fingerprinted with ("aac" or "libfdk_aac"); empty → native. Same
	// contract as HLSProcessOpts.AudioEncoder: one value for the bytes AND the
	// hash, so they cannot disagree.
	AudioEncoder string

	// VideoCompositionDelay is the seconds by which the VIDEO's first frame is
	// presented late - has_b_frames/fps, because MP4 forbids a negative DTS. The
	// muxed pipeline shifted audio and video together; a separate audio rendition
	// starts at 0 and would therefore run EARLY by exactly this much (measured:
	// 42 ms for 1 B-frame, 83 ms for 2, at 23.976 fps). Past the ~45 ms threshold
	// for early audio, that is audible. 0 = nothing to compensate.
	VideoCompositionDelay float64

	// ReadRate paces ffmpeg's input to N× real-time (`-readrate N`, pre-`-i`) so a
	// single long-lived audio rendition doesn't decode-and-re-encode the WHOLE
	// remaining track flat-out the instant it starts (and again on every seek).
	// The AC-3/E-AC-3→AAC chain is CPU-bound; UNPACED it bursts one full core per
	// client on a weak NAS. Pacing spreads that fixed work across playback instead
	// of a startup spike. `-readrate` gates INPUT reading only - the encoded output
	// is byte-identical, so the audio segment cache/fingerprint is unaffected.
	// 0 (default) = no pacing, byte-for-byte and CPU-profile identical to before.
	// Paired with a full-speed initial burst (appendCopyPacingBurst) so cold start
	// and seeks still fill the player's buffer instantly before the cap engages.
	ReadRate float64

	// SilenceDurationSec > 0 turns the session into a bounded SILENCE generator:
	// the input becomes `-f lavfi -i anullsrc` (FilePath is ignored) producing
	// SilenceDurationSec seconds of silent audio, segmented and numbered exactly
	// like a real session (StartNumber/StartSec still apply for the fMP4 rebase).
	// Purpose: an audio track that ends BEFORE the container's declared duration
	// leaves the VOD playlist promising segments no -ss on the source can yield;
	// the generator keeps that promise so playback just continues through the
	// silent tail instead of dying on missing segments. The bound is mandatory -
	// an unbounded anullsrc would encode forever.
	SilenceDurationSec float64
}

// HLSProcessHandle controls a running HLS transcode.
type HLSProcessHandle interface {
	WaitReady(ctx context.Context) error
	Stop() error
	Done() <-chan struct{}
	Err() error
	// Failure classifies WHY the process exited, for the level-2 HW safety net.
	// Only meaningful after Done() has closed; reports FailureNone before that.
	// The orchestrator reads it on natural exit to decide whether to demote the
	// host's HW capability (a HW-capability class) or ignore it (cancel/transient).
	Failure() FailureClass
	// Pause suspends the transcode process (SIGSTOP) so it stops advancing
	// the sliding window while the client isn't pulling segments. Resume
	// (SIGCONT) lets it continue. Both are idempotent and no-op when the
	// process has already exited.
	Pause() error
	Resume() error
}

type HLSProcessOpts struct {
	OutputDir          string
	FilePath           string
	Profiles           []HLSProfile
	VideoDecision      string // "copy", "transcode", or "" (multi-variant)
	AudioDecision      string // "copy" or "transcode"
	AudioTrackIndex    int
	SubtitleTrackIndex int // -1 = no subtitle
	SubtitleCodec      string
	HDRSupported       bool
	// ToneMap forces an HDR->SDR tonemap filter even when Streams is not threaded
	// in. The orchestrated VOD path passes the tonemap decision as this bool (from
	// DecisionFingerprint.HDRTonemap) rather than the source streams, so the
	// builder honours it independently of stream-based detection. Ignored when
	// HDRSupported is true (the client renders HDR natively -> never tonemap).
	ToneMap bool
	// HWTonemap, when true, makes the HARDWARE builder tone-map HDR->SDR on the
	// GPU (tonemap_vaapi etc.) as part of the zero-copy pipeline, instead of the
	// software zscale path. Set only when the host probed HWTonemap, the toggle is
	// on, and the content goes the HW path; mutually exclusive with the software
	// ToneMap above (the HW builder ignores ToneMap, the software builder ignores
	// this). When false the software path is byte-identical to a no-HW-tonemap host.
	HWTonemap bool
	// DeinterlaceAll switches the software deinterlacer from bwdif
	// deint=interlaced (flag-gated) to deint=all, for legacy sources whose
	// container flags lie (repacked DVD rips store interlaced fields as
	// "progressive" frames → combing the flag gate never touches). Set from
	// the idet content-detection verdict (DecisionFingerprint.DeinterlaceAll).
	// false = the unchanged flag-gated behaviour, a no-op on real progressive.
	DeinterlaceAll bool
	// SourceSAR is the source sample aspect ratio ("64:45" on anamorphic DVD),
	// empty/"1:1" for square pixels. When anamorphic, the builder derives the
	// scale height from the DISPLAY aspect so the output isn't stretched -
	// the historical scale=W:-2,setsar=1 preserved storage pixels and squashed
	// the picture.
	SourceSAR      string
	AudioNormalize bool
	StartSec       float64
	// StartNumber is the absolute segment index this process starts producing at
	// (ffmpeg -start_number). Only the COPY builder reads it: keyframe-aligned copy
	// segments are variable-length, so the time→index map is non-linear and
	// start_number cannot be derived from StartSec/segLen - a forward seek to
	// plan-segment K must emit stream_N_<K>, not stream_N_<floor(StartSec/segLen)>.
	// Transcode (uniform segments) ignores it and derives start_number from StartSec.
	StartNumber int
	// CopyOnSeek keeps -c:v copy on a mid-stream start (StartSec>0) instead of the
	// libx264 re-encode the legacy remux used for seeks. The copy rung sets it: its
	// StartSec is always a plan keyframe boundary, so input-seek + copy lands
	// exactly AND preserves the source avcC. Without it a seek re-encodes to a
	// libx264 Baseline init that overwrites the shared High-profile init →
	// every copied segment decodes against the wrong SPS/PPS → scrambled frames.
	CopyOnSeek bool
	// SeekTargetSec is the EXACT `-ss` value a copy-on-seek session must emit.
	// It is the timestamp the producer's seek-landing probe ran against, so by
	// construction ffmpeg lands where the probe said (StartSec/StartNumber are
	// set to that landing). Deriving -ss from the landed StartSec instead would
	// re-trigger the landing rules one keyframe earlier every time (the ffmpeg
	// CLI subtracts ~130 ms before seeking when the video has B-frames, so
	// aiming AT a cue point always lands BEFORE it). 0 → fall back to
	// StartSec + CopySeekNudgeSec (no probe ran).
	SeekTargetSec float64
	// ExpectedSegmentStartMs reports the keyframe plan's start time for an
	// absolute copy-segment index - the runtime tripwire's oracle. The copy
	// publish pump compares every produced segment's (rebased) timeline
	// against it and raises an ERROR log + metric on divergence, so a future
	// regression of the plan↔ffmpeg contract (split grid, seek landing, audio
	// trim labeling - each bit us once) trips an alert instead of an audible
	// desync. nil = no verification. Second return false = index unknown.
	ExpectedSegmentStartMs func(segIdx int) (int64, bool)
	// TimelineDriftKind labels which drift metric the tripwire increments when
	// ExpectedSegmentStartMs diverges: "copy" (the keyframe-plan rung) or
	// "transcode_seek" (a mid-stream transcode session, whose fixed-length
	// segments give a linear oracle). Empty = "copy" for back-compat.
	TimelineDriftKind string
	// ReadRate paces ffmpeg's input to N× real-time (`-readrate N`, a pre-`-i`
	// flag) so a single long-lived encoder stays close to wall-clock and doesn't
	// burn CPU racing ahead of the viewer. 0 (default) = no pacing - byte-identical
	// to today. Only the single-ffmpeg SOFTWARE producer sets it (≈1.5); the HW
	// orchestrator leaves it 0 and relies on its burst-ahead + SIGSTOP throttle.
	ReadRate float64
	Streams  []*library.MediaStream

	// ProgressLogPath, when non-empty, makes this session write ffmpeg's own
	// `-progress` stream (frame, fps, out_time, speed, every ~0.5 s) to that
	// file. Empty = off, and off is the default: it is one file per encoder on
	// a box that may serve thousands of playbacks, so it is a DEBUG facility an
	// operator turns on (transcode.ffmpeg_progress_log), not a running cost.
	//
	// Why it exists: everything else about a session is logged at DECISION time
	// (the argv, the spawn, the kill, the reach verdict), which says what the
	// server MEANT to do. This is the only record of what ffmpeg actually
	// achieved second by second. On 2026-09-06 its absence is precisely what
	// left "the encoder was too far behind when the client hung up" a deduction
	// rather than a measurement - the stderr ring buffer is 8 KB, overwritten
	// continuously, and read only when a session FAILS.
	ProgressLogPath string

	// AudioEncoder names the CONCRETE ffmpeg AAC implementation this session was
	// fingerprinted with ("aac" or "libfdk_aac"). Empty → the native encoder,
	// i.e. the historical bytes.
	//
	// It rides the OPTS rather than being probed inside the builder on purpose:
	// the very same string is hashed into the segment cache key. Two encoders at
	// one bitrate make different bytes, so a cache that could not tell them apart
	// would hand a player the first half of a session from one and the second
	// half from the other after an image swap. One value, set once per session,
	// read by both the command line and the hash - they cannot disagree.
	AudioEncoder string

	// SourceVideoCodec is the codec of the SOURCE video stream ("h264",
	// "hevc"…). The COPY builder reads it to tag the fMP4 sample entry
	// (`-tag:v hvc1` for HEVC - Safari refuses ffmpeg's hev1 default);
	// transcode builders ignore it (they know their encoder). Empty → no tag
	// override (h264 copy needs none).
	SourceVideoCodec string

	// Overrides propagated from the playback decision. Empty / zero values
	// preserve the previous default behavior (libx264 + aac + 2-channel + burn-in
	// for bitmap subs).
	VideoCodec       string // domain codec name: "h264"|"hevc"/"h265"|"av1". Empty → libx264.
	AudioCodec       string // domain codec name: "aac"|"ac3"|"eac3"|"opus". Empty → aac.
	MaxAudioChannels int    // 0 = no cap (preserves current -ac 2 default when transcoding audio).
	ForceSidecarSubs bool   // true → skip burn-in even for bitmap subs (served as sidecar externally).

	// HWAccel selects the hardware encoder backend. None/"" → software
	// (libx264/libx265/libaom-av1, the unchanged default). When set, the
	// ffmpeg builder emits a zero-copy hardware pipeline (hwaccel decode →
	// HW scale → HW encode). The wiring layer only sets this to a non-None
	// value when (a) the flag transcoding.hw_accel_enabled is on, (b) a
	// detected device supports the requested codec, and (c) the content is
	// simple (no HDR tonemap, no bitmap-subtitle burn-in) - otherwise the
	// builder falls back to the software path for correctness.
	HWAccel HWAccelType
	// HWDevice is the DRM render node for VAAPI/QSV (e.g. /dev/dri/renderD128).
	// Ignored by NVENC (CUDA selects the device). Empty → ffmpeg default.
	HWDevice string
	// HWFeatures is the per-option tuning set the calibration probe confirmed
	// the HW encoder ACCEPTS on this host (capability.HWFeatures). The builder
	// emits an optional tuning flag only when its name is present, so an option
	// an older chip rejects is never passed. Empty → the conservative HW
	// baseline (preset + NVENC's required -forced-idr). Clearing this is also
	// the level-2 "HW-minimal" retry tier.
	HWFeatures []string

	// UseFMP4 picks the segment container: true → fMP4 (.m4s + per-variant
	// init segment), false → MPEG-TS (.ts). Must match the extension baked
	// into the manifest the player consumes - otherwise segment fetches
	// look up a file the encoder never produces and time out forever.
	// Single-variant builders already honoured this via appendHLSOutputArgs;
	// the multi-variant builder used to hardcode .ts and silently ignored
	// the project's fMP4 default flip.
	UseFMP4 bool

	// VariantIndex is the ABR rung this process produces (stream_<N>_*). It only
	// matters for single-output builders: the multi-variant builder names every
	// rung itself via -var_stream_map. Default 0 keeps the historical single-
	// variant naming. The per-variant HW path sets it so each of the N
	// single-output ffmpeg processes writes its own rung.
	VariantIndex int

	// PerVariantHW, when true AND HWAccel is set AND there is more than one
	// profile, makes the transcoder spawn ONE single-output ffmpeg per variant
	// (behind one composite handle) instead of a single multi-output process.
	// This matches how mature media servers drive HW (avoids the multi-encode-in-one-
	// process graph that crashes fragile VA drivers and isolates per-rung
	// failures). Software always uses the single-process multi-variant graph.
	PerVariantHW bool

	// AudioUserOffsetSec is the USER's audio resync request (seconds; positive
	// = audio later), threaded from the decision fingerprint. Combined with
	// AudioStartDelaySec below into one net input shift by the builders -
	// transcoded audio only, like the source-delay cancel.
	AudioUserOffsetSec float64

	// AudioStartDelaySec is how far the SOURCE's selected audio track starts
	// AFTER its video track (ffprobe start_time delta, seconds). Some repacks
	// carry a large one (~1s) as a mux defect; the pipeline's PTS preservation
	// (-copyts and friends) reproduces it verbatim as a constant A/V desync.
	// When it exceeds the builder's floor AND the audio is transcoded, the
	// builders cancel it by feeding audio from a second `-itsoffset -<delay>`
	// input. The transcoder adapter probes and sets this at session start
	// (HLSTranscoder.Start) so BuildFFmpegArgs stays pure; 0 (default) is a
	// strict no-op. Copied audio is never delay-cancelled (its packets cannot
	// be trimmed at the seek edge; see the builder tests).
	AudioStartDelaySec float64
}

// Validate rejects an HLSProcessOpts that cannot produce a usable ffmpeg
// invocation. It exists because this struct has grown past 35 fields fed from
// three producers: without it, a missing path or a typo'd decision surfaces as
// a dead ffmpeg process and a cryptic exit code minutes later, blamed on the
// media rather than on the caller.
//
// It checks only what is ALWAYS wrong - required inputs and closed value sets.
// The subtler combinations stay documented on the fields rather than enforced
// here: ToneMap vs HWTonemap for instance reads as mutually exclusive, but the
// two builders each ignore the other's flag, so a request carrying both is
// served correctly today and rejecting it would break a working call.
//
// A deliberate NON-constructor: an ADR-0008-style NewX + Options builder was
// considered and dropped - there are two construction sites, both filling the
// struct field by field from a fingerprint, and 35 With* options would be more
// code to keep in sync than the literal they replace.
func (o HLSProcessOpts) Validate() error {
	if o.FilePath == "" {
		return errors.New("hls opts: source FilePath is required")
	}
	if o.OutputDir == "" {
		return errors.New("hls opts: OutputDir is required")
	}
	if len(o.Profiles) == 0 {
		return errors.New("hls opts: at least one profile is required")
	}
	switch o.VideoDecision {
	case "", "copy", "transcode":
	default:
		return fmt.Errorf("hls opts: invalid VideoDecision %q", o.VideoDecision)
	}
	switch o.AudioDecision {
	case "", "copy", "transcode":
	default:
		return fmt.Errorf("hls opts: invalid AudioDecision %q", o.AudioDecision)
	}
	if o.StartSec < 0 {
		return fmt.Errorf("hls opts: negative StartSec %v", o.StartSec)
	}
	if o.StartNumber < 0 {
		return fmt.Errorf("hls opts: negative StartNumber %d", o.StartNumber)
	}
	// A copy-on-seek session that is not a video copy would hand `-c:v copy` to
	// a rung the manifest advertises as re-encoded.
	if o.CopyOnSeek && o.VideoDecision != "copy" {
		return fmt.Errorf("hls opts: CopyOnSeek requires VideoDecision \"copy\", got %q", o.VideoDecision)
	}
	return nil
}
