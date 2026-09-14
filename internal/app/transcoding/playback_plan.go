package transcoding

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The HLS read path's brain lives in PlanResolver (playback_plan_resolver.go):
// each request is resolved ONCE into an immutable PlaybackPlan the http
// adapter serves off, so manifest- and segment-time read one resolution
// (docs/playback-serving.md). Across requests the invariant rests on URL
// baking (querySuffix) plus the shaper's frozen ladder / StreamPlan pins.

// PlaybackRequest is the transport-free input to plan resolution: source
// facts, client capabilities, profile preferences and the request-time pins.
type PlaybackRequest struct {
	MediaID    string
	SourcePath string
	Container  string
	Streams    []*library.MediaStream
	Client     transcoding.ClientProfile
	Prefs      TranscodingPreferences

	// Request-time pins - baked into the child URLs so a mid-play playlist poll
	// cannot flip the plan under the player (the cache-hash invariant).
	MaxBitrateKbps      int
	AudioTrack          int
	SubtitleBurnInTrack int
	AudioOffsetMs       int
	PinnedVideoDecision string // "", "copy", or "transcode" (the vd= URL pin)
	PinnedAudioDecision string // "", "copy", or "transcode" (the ad= URL pin)
	// PinnedVideoCodec (the vc= URL pin) fixes the OUTPUT codec of a transcode:
	// the copy master's fallback rung pins the copy rung's codec so a starving
	// player never switches codec family between the two variants. Honoured
	// only when the client decodes it; never upgraded to HEVC by the resolver.
	PinnedVideoCodec string
	WasmAudio           bool   // the ?wasmAudio=1 client opt-in for browser AC-3 decode

	// SourceBytes is the on-disk size of the source file (0 = unknown), read
	// by the copy-rung disk guard - fail-open on 0.
	SourceBytes int64
	// SourceBitrateKbps is the video stream's bitrate (else the file's); the served ladder follows it.
	SourceBitrateKbps int64
	// ScreenHeight is the viewer's physical screen height (?screenHeight); rungs
	// above the one covering it are dropped. 0 = unknown.
	ScreenHeight int
}

// PackagerKind names which engine produces the media bytes for a plan - the
// cost axis as a queryable property of the plan.
type PackagerKind string

const (
	// PackagerCopyMkvgo serves the copy rung straight from the container with the
	// mkvgo on-demand packager: no ffmpeg process, no video encode. Free.
	PackagerCopyMkvgo PackagerKind = "mkvgo-copy"
	// PackagerCopyFFmpeg remuxes via the ffmpeg copy producer (-c:v copy): a
	// process, but no video re-encode. Used when the mkvgo plan cannot build.
	PackagerCopyFFmpeg PackagerKind = "ffmpeg-copy"
	// PackagerTranscodeFFmpeg re-encodes the video with ffmpeg. Costly.
	PackagerTranscodeFFmpeg PackagerKind = "ffmpeg-transcode"
)

// PlaybackPlan is the immutable, fully-resolved answer to "how do we serve this
// stream". Emitted once and read verbatim; a capacity/config change only
// affects plans emitted after it.
type PlaybackPlan struct {
	// Decision is the per-track copy/transcode decision with the request pins applied.
	Decision transcoding.PlaybackDecision
	// Class is the ladder class the video encode is billed as (cpu vs gpu pool).
	Class LadderClass
	// Ladder is the shaped ABR ladder, frozen: every cap and the admission
	// ceiling folded in. A copy rung carries its single source-resolution profile.
	Ladder []transcoding.HLSProfile
	// VideoCodec is what the segments actually carry (AV1 already gated back to
	// h264 when unsupported); HWAccel is the resolved hardware route ("" = software).
	VideoCodec string
	HWAccel    string
	// Packager is which engine writes the bytes - see PackagerKind.
	Packager PackagerKind
	// FingerprintHash is the DecisionFingerprint.Hash() addressing the cache and
	// the ffmpeg session for this plan.
	FingerprintHash string
	// Fingerprint is the full cache-key object behind FingerprintHash, handed to
	// the producers.
	Fingerprint DecisionFingerprint
	// WasmAudio is true when the AC-3/E-AC-3 audio is served as an out-of-band
	// rendition for the browser wasm decoder (the master is video-only).
	WasmAudio bool
}

// NeedsVideoEncode reports whether serving this plan spends encode CPU/GPU (a real
// ffmpeg transcode) rather than a copy. The admission limiter only bills these.
func (p PlaybackPlan) NeedsVideoEncode() bool {
	return p.Packager == PackagerTranscodeFFmpeg
}

// UsesFFmpeg reports whether serving this plan spawns any ffmpeg process at all
// (transcode, or the ffmpeg copy remux) - i.e. it is NOT the zero-process mkvgo path.
func (p PlaybackPlan) UsesFFmpeg() bool {
	return p.Packager != PackagerCopyMkvgo
}

// PlaybackPlanResolver turns a marshalled PlaybackRequest into an immutable
// PlaybackPlan; the http adapter depends only on this interface.
type PlaybackPlanResolver interface {
	Resolve(ctx context.Context, req PlaybackRequest) (PlaybackPlan, error)
}
