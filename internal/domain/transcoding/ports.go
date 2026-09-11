package transcoding

import (
	"context"
	"errors"
	"fmt"
)

// SourceStream is the slice of a probed stream the HLS process reads (the
// colour metadata behind the HDR -> SDR tonemap decision, shared with
// internal/pkg/hdr). An interface so this context does not import the library
// domain (ADR-0001); *library.MediaStream satisfies it as is.
type SourceStream interface {
	GetStreamType() string
	GetBitDepth() *int
	GetColorSpace() *string
	GetColorTransfer() *string
	GetHDRType() *string
	GetProfile() string
	GetDoViProfile() *int
	GetDoViBLCompat() *int
}

type Transcoder interface {
	Start(ctx context.Context, job *TranscodeJob) (string, error)
	Stop(ctx context.Context, jobID string) error
	Progress(ctx context.Context, jobID string) (*TranscodeProgress, error)
	// Pause (SIGSTOP) / Resume (SIGCONT) let live playback pre-empt background jobs.
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

// CopySeekNudgeSec is added to a copy session's pre-input -ss: StartSec comes
// from the ms-rounded keyframe index and can sit a hair below the keyframe,
// and -ss seeks at-or-before, so an exact target lands one keyframe early
// (jellyfin-ffmpeg7 harness: 1 ms below -> previous keyframe, +50 ms -> correct).
// Shared by the arg builder and the copy producer's seek-landing probe.
const CopySeekNudgeSec = 0.05

// AudioProcess spawns an ffmpeg audio-only HLS rendition process
// (audio_T_init.mp4 + audio_T_NNN.m4s, served as an EXT-X-MEDIA rendition).
type AudioProcess interface {
	StartAudioOnly(ctx context.Context, opts AudioProcessOpts) (HLSProcessHandle, error)
}

// AudioProcessOpts configures one audio-only HLS rendition encode; T in the
// filenames is SourceTrackIdx.
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
	// AudioEncoder is the concrete AAC implementation the rendition was
	// fingerprinted with ("aac" / "libfdk_aac"); empty = native. One value for
	// the bytes and the hash, like HLSProcessOpts.AudioEncoder.
	AudioEncoder string

	// VideoCompositionDelay is how late (s) the video's first frame is presented
	// (has_b_frames/fps: MP4 forbids a negative DTS). A separate audio rendition
	// starts at 0 and would run early by that much - audible past ~45 ms.
	VideoCompositionDelay float64

	// ReadRate paces the input (`-readrate N`) so a long-lived rendition does not
	// re-encode the whole remaining track flat-out at start and on every seek.
	// Output bytes are unchanged. 0 = no pacing.
	ReadRate float64

	// SilenceDurationSec > 0 makes the session a bounded silence generator
	// (`-f lavfi -i anullsrc`, FilePath ignored), numbered like a real session:
	// an audio track ending before the container's declared duration would
	// otherwise leave the playlist promising segments no -ss can yield.
	SilenceDurationSec float64
}

// HLSProcessHandle controls a running HLS transcode.
type HLSProcessHandle interface {
	WaitReady(ctx context.Context) error
	Stop() error
	Done() <-chan struct{}
	Err() error
	// Failure classifies WHY the process exited (the level-2 HW safety net);
	// FailureNone until Done() has closed.
	Failure() FailureClass
	// Pause (SIGSTOP) / Resume (SIGCONT) are idempotent and no-ops once exited.
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
	// ToneMap forces the HDR->SDR tonemap without threaded Streams (the VOD
	// path passes the decision as a bool). Ignored when HDRSupported.
	ToneMap bool
	// HWTonemap makes the HARDWARE builder tone-map on the GPU; set only when
	// the host probed it, the toggle is on and the content takes the HW path.
	HWTonemap bool
	// HDRPassthrough: HW transcode to HEVC Main10 keeping the HDR (HDRTransfer "pq"/"hlg"), no tone-map.
	HDRPassthrough bool
	HDRTransfer    string
	// HWOverlay lets the HARDWARE builder blend the bitmap subtitle on the GPU;
	// false keeps a burn-in on the software path.
	HWOverlay bool
	// DeinterlaceAll switches bwdif from deint=interlaced to deint=all for
	// sources whose container flags lie (idet verdict, DecisionFingerprint.DeinterlaceAll).
	DeinterlaceAll bool
	// InverseTelecine replaces the deinterlacer with fieldmatch + decimate for a
	// 3:2 pulldown source (DecisionFingerprint.Telecine). Software path only.
	InverseTelecine bool
	// SourceSAR ("64:45" on an anamorphic DVD, empty/"1:1" square): the scale
	// height derives from the DISPLAY aspect so the picture is not squashed.
	SourceSAR string
	// MaxH264Level caps the level libx264 signals (client decoder cap), 0 = none.
	MaxH264Level   int
	AudioNormalize bool
	StartSec       float64
	// StartNumber is the absolute segment index this process starts at (ffmpeg
	// -start_number). Only the COPY builder reads it: keyframe-cut segments are
	// variable-length, so the index cannot be derived from StartSec.
	StartNumber int
	// CopyOnSeek keeps -c:v copy on a mid-stream start: StartSec is a plan
	// keyframe, so input-seek + copy lands exactly and keeps the source avcC (a
	// libx264 re-encode overwrote the shared High init with a Baseline one).
	CopyOnSeek bool
	// SeekTargetSec is the exact -ss a copy-on-seek session emits: the timestamp
	// the seek-landing probe ran against, so ffmpeg lands where the probe said.
	// Deriving it from StartSec lands one keyframe earlier on B-frame sources
	// (the ffmpeg CLI's ~130 ms DTS heuristic). 0 = StartSec + CopySeekNudgeSec.
	SeekTargetSec float64
	// ExpectedSegmentStartMs is the keyframe plan's start time per absolute
	// segment index - the publish pump's tripwire compares every produced
	// segment against it (metric + ERROR on drift). nil = no verification.
	ExpectedSegmentStartMs func(segIdx int) (int64, bool)
	// TimelineDriftKind selects the drift metric: "copy" (default) or "transcode_seek".
	TimelineDriftKind string
	// ReadRate paces the input (`-readrate N`) so a long-lived software encoder
	// stays near wall-clock. 0 = no pacing (the HW orchestrator self-throttles).
	ReadRate float64
	Streams  []SourceStream

	// ProgressLogPath, when set, writes ffmpeg's `-progress` stream there: the
	// only second-by-second record of what ffmpeg achieved (stderr is an 8 KB
	// ring read only on failure). A debug facility (transcode.ffmpeg_progress_log).
	ProgressLogPath string

	// AudioEncoder is the concrete AAC implementation this session was
	// fingerprinted with ("aac" / "libfdk_aac"); empty = native. Rides the opts
	// because the same string is hashed into the cache key: the command line
	// and the hash cannot disagree.
	AudioEncoder string

	// SourceVideoCodec lets the COPY builder tag the fMP4 sample entry
	// (`-tag:v hvc1` for HEVC, Safari refuses hev1). Transcode builders ignore it.
	SourceVideoCodec string

	// Overrides from the playback decision; zero values keep the historical
	// defaults (libx264 + aac + 2 channels + burn-in for bitmap subs).
	VideoCodec       string // domain codec name: "h264"|"hevc"/"h265"|"av1". Empty → libx264.
	AudioCodec       string // domain codec name: "aac"|"ac3"|"eac3"|"opus". Empty → aac.
	MaxAudioChannels int    // 0 = no cap (preserves current -ac 2 default when transcoding audio).
	ForceSidecarSubs bool   // true → skip burn-in even for bitmap subs (served as sidecar externally).

	// HWAccel selects the hardware backend (None/"" = software). Set by the
	// wiring only when the flag is on and a probed device encodes the codec;
	// the builder still falls back to software for what it cannot express.
	HWAccel HWAccelType
	// HWDevice is the DRM render node for VAAPI/QSV; ignored by NVENC.
	HWDevice string
	// HWFeatures is the tuning set the calibration probe confirmed the encoder
	// accepts; a flag is emitted only when named here. Empty = baseline.
	HWFeatures []string
	// HWSoftwareDecode: the DECOUPLED route - software decode + hwupload, GPU
	// scale/encode - for a source the GPU cannot decode but can encode (MPEG-2,
	// MPEG-4 ASP rips next to an idle encoder). VAAPI/QSV only, the chains the
	// calibration validates; NVENC keeps the full-software fallback.
	HWSoftwareDecode bool

	// UseFMP4 picks the segment container (.m4s + init vs .ts); it must match
	// the extension the manifest advertises or fetches time out forever.
	UseFMP4 bool

	// VariantIndex is the ABR rung a single-output process produces
	// (stream_<N>_*); the multi-variant builder names every rung itself.
	VariantIndex int

	// PerVariantHW spawns one single-output ffmpeg per rung on the HW path
	// (behind one composite handle): the multi-encode graph crashes fragile VA
	// drivers, and a rung's failure should not take the others down.
	PerVariantHW bool

	// AudioUserOffsetSec is the user's audio resync (s, positive = audio later);
	// combined with AudioStartDelaySec into one net input shift (transcoded audio only).
	AudioUserOffsetSec float64

	// AudioStartDelaySec is how far the selected audio track starts after the
	// video (a mux defect on some repacks, ~1 s). Above the builder's floor and
	// on transcoded audio, the builders cancel it with a second `-itsoffset`
	// input. Set by HLSTranscoder.Start so BuildFFmpegArgs stays pure.
	AudioStartDelaySec float64
}

// Validate rejects opts that can never produce a usable ffmpeg invocation
// (required inputs, closed value sets): a missing path otherwise surfaces
// minutes later as a dead process blamed on the media. Subtler combinations
// (ToneMap vs HWTonemap) are documented on the fields, not enforced.
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
