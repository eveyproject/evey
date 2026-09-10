package transcoding

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// THE SEAM IS WIRED. See docs/playback-serving.md.
//
// The request-handling brain of the HLS read path - the copy/transcode
// decision plus its request-time pins, the ABR ladder shaping, the admission
// route/ceiling, the decision fingerprint, and the packager routing - lives in
// PlanResolver (playback_plan_resolver.go), the concrete PlaybackPlanResolver.
// Each request is resolved ONCE:
//
//   PlaybackPlanResolver.Resolve(req) -> immutable PlaybackPlan
//
// The http adapter marshals the *http.Request into a PlaybackRequest
// (http/transcode_vod_plan.go: marshalPlaybackRequest + resolvePlan) and
// serves off the returned plan; manifest- and segment-time read the same
// resolution, so within a request the URLs<->bytes invariant holds by
// construction (one resolution, not N that must agree). Wiring a new
// codec/packager/container is one change in the resolver, not a sweep across
// the adapter. Across requests the invariant still rests on URL baking
// (querySuffix) plus the shaper's frozen ladder / StreamPlan pins.

// PlaybackRequest is the marshalled, transport-free input to plan resolution:
// everything the http adapter reads off the *http.Request and the media, with no
// net/http type crossing into the app layer. It carries the source facts (media,
// streams, container), the client's declared capabilities, the profile
// preferences, and the request-time PINS that today live scattered in
// makeDecision / buildFingerprint (the vd=/ad= decision pins, the selected audio
// track, the burned-in subtitle, the audio resync offset, the wasm-audio opt-in).
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
	WasmAudio           bool   // the ?wasmAudio=1 client opt-in for browser AC-3 decode

	// SourceBytes is the on-disk size of the source file (0 = unknown), read
	// by the copy-rung disk guard - fail-open on 0.
	SourceBytes int64
	// SourceBitrateKbps is the file's overall bitrate; the served ladder follows it.
	SourceBitrateKbps int64
	// ScreenHeight is the viewer's physical screen height (?screenHeight); rungs
	// above the one covering it are dropped. 0 = unknown.
	ScreenHeight int
}

// PackagerKind names which engine produces the media bytes for a plan - the
// cost axis made a first-class, queryable property of the plan (see the helper
// methods) instead of something a reader has to reconstruct from the decision.
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

// PlaybackPlan is the IMMUTABLE, fully-resolved answer to "how do we serve this
// stream": the per-track decision, the shaped ABR ladder and its ladder class,
// the effective (post-gate) video codec and hw route, the packager that will
// produce the bytes, and the decision fingerprint hash that keys the cache. It is
// emitted once and read verbatim; a capacity/config change only affects plans
// emitted after it (like StreamPlan, which this subsumes).
type PlaybackPlan struct {
	// Decision is the per-track copy/transcode decision (with the request pins
	// already applied), the single source of truth the packager and manifest read.
	Decision transcoding.PlaybackDecision
	// Class is the ladder class the video encode is billed as (cpu vs gpu pool).
	Class LadderClass
	// Ladder is the shaped ABR ladder, frozen: source/capability/CMCD/av1 caps and
	// the admission ceiling already folded in. A copy rung carries its single
	// source-resolution profile (the player never ABR-switches a copy stream).
	Ladder []transcoding.HLSProfile
	// VideoCodec is what the segments actually carry (AV1 already gated back to
	// h264 when unsupported); HWAccel is the resolved hardware route ("" = software).
	VideoCodec string
	HWAccel    string
	// Packager is which engine writes the bytes - see PackagerKind.
	Packager PackagerKind
	// FingerprintHash is the DecisionFingerprint.Hash() addressing the cache and
	// the ffmpeg session for this plan. The stable identity every segment resolves.
	FingerprintHash string
	// Fingerprint is the full cache-key object behind FingerprintHash: the
	// serving path still hands it to the producers (EnsureSegmentRange,
	// Manifest) and reads per-rung facts (Profiles, UseFMP4) off it.
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
// (transcode, or the ffmpeg copy remux) - i.e. it is NOT the zero-process mkvgo
// path. The cost axis of the whole read pipeline, in one predicate.
func (p PlaybackPlan) UsesFFmpeg() bool {
	return p.Packager != PackagerCopyMkvgo
}

// PlaybackPlanResolver is the PROPOSED app-layer use case that turns a marshalled
// PlaybackRequest into an immutable PlaybackPlan - the single entry point the http
// adapter would call in place of the scattered makeDecision/profilesForDecision/
// buildFingerprint dance. A concrete resolver lives in the app layer (where the
// decision engine, calibration, admission and cost model already are); the http
// adapter depends only on this interface.
type PlaybackPlanResolver interface {
	Resolve(ctx context.Context, req PlaybackRequest) (PlaybackPlan, error)
}
