package transcoding

import (
	"context"

	// library.MediaStream is used in HLSProcessOpts. Duplicating it here would
	// be worse: the struct has 20+ fields, is referenced in 24 files, and
	// transcoding inherently depends on stream metadata (codec, resolution, HDR).
	"github.com/evey-project/evey/internal/domain/library"
)

type Transcoder interface {
	Start(ctx context.Context, job *TranscodeJob) (string, error)
	Stop(ctx context.Context, jobID string) error
	Progress(ctx context.Context, jobID string) (*TranscodeProgress, error)
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
}

// HLSProcessHandle controls a running HLS transcode.
type HLSProcessHandle interface {
	WaitReady(ctx context.Context) error
	Stop() error
	Done() <-chan struct{}
	Err() error
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
	AudioNormalize     bool
	StartSec           float64
	Streams            []*library.MediaStream

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
	// simple (no HDR tonemap, no bitmap-subtitle burn-in) — otherwise the
	// builder falls back to the software path for correctness.
	HWAccel HWAccelType
	// HWDevice is the DRM render node for VAAPI/QSV (e.g. /dev/dri/renderD128).
	// Ignored by NVENC (CUDA selects the device). Empty → ffmpeg default.
	HWDevice string

	// UseFMP4 picks the segment container: true → fMP4 (.m4s + per-variant
	// init segment), false → MPEG-TS (.ts). Must match the extension baked
	// into the manifest the player consumes — otherwise segment fetches
	// look up a file the encoder never produces and time out forever.
	// Single-variant builders already honoured this via appendHLSOutputArgs;
	// the multi-variant builder used to hardcode .ts and silently ignored
	// the project's fMP4 default flip.
	UseFMP4 bool
}
