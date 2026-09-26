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
// PlaybackDecider answers the plan's decision for a request that will not be
// served by the caller (the pre-play info surfaces): Decide with the
// request-time pins, no route, ladder, pin or fingerprint behind it.
// *PlanResolver is the production decider.
type PlaybackDecider interface {
	Decide(ctx context.Context, req PlaybackRequest) transcoding.PlaybackDecision
}

// UnwiredDecider decides without a host: no calibration, no copy producer,
// the operator's toggles unread. What an info surface answers before the
// serving pipeline is wired (tests, a server without the HLS pipeline).
func UnwiredDecider() PlaybackDecider { return &PlanResolver{} }

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
	WasmAudio        bool // the ?wasmAudio=1 client opt-in for browser AC-3 decode

	// SourceBytes is the on-disk size of the source file (0 = unknown), read
	// by the copy-rung disk guard - fail-open on 0.
	SourceBytes int64
	// SourceDurationMs is the container duration the manifests declare; the
	// fingerprint carries it to the encoder (DecisionFingerprint.SourceDurationMs).
	SourceDurationMs int64
	// SourceBitrateKbps is the video stream's bitrate (else the file's); the served ladder follows it.
	SourceBitrateKbps int64
	// SourceNeedsRemux: the file is stored in one-track blocks (MediaFile.NeedsRemux);
	// the decision routes it to the segmented session where the typed shed is met.
	SourceNeedsRemux bool
	// Source is the snapshot of the facts above, taken once by Resolve
	// (SnapshotOf). Callers leave it zero; a sub-derivation called on its own
	// (the adapter's wrappers, tests) takes it on demand.
	Source SourceSnapshot
	// Policy is the operator's hardware policy this request is resolved
	// under, read once by Resolve (hwPolicy) like Source.
	Policy HWPolicy
	// ScreenHeight is the viewer's physical screen height (?screenHeight); rungs
	// above the one covering it are dropped. 0 = unknown.
	ScreenHeight int

	// LineKbps is the client's own throughput measurement (?maxKbps=, the cast
	// receiver's bitrate test, its network stack included) and ABRFloor its
	// starvation rescue (?abr=floor). Constraints on a COPY plan: a copy the
	// line cannot carry, or a rescue, fits the plan to ONE transcoded rung
	// (PlaybackPlan.Fit) - see resolveLineConstrained. Inert on a request
	// whose video decision is already pinned (vd=): the master judged them.
	LineKbps int64
	ABRFloor bool
	// SourcePeakKbps is what a copy pushes down the line: the file's bitrate
	// (video and audio), the figure LineKbps is measured against. 0 = unknown.
	SourcePeakKbps int64
	// fitReason names, on the fitted pass of resolveLineConstrained, why the
	// video decision is pinned to a transcode; Decide stamps it instead of
	// decision_pinned. Never set by a caller.
	fitReason string
}

// LineFit narrows the master to ONE rung of the ladder: the client's line
// could not carry the copy (LineKbps) or its starvation rescue asked for the
// floor (ABRFloor). The ladder and the fingerprint stay the full transcode
// ones - the child URLs index the ladder and a viewer of the same title
// shares its sessions and cache - only the master's presentation narrows.
// Zero = the whole ladder.
type LineFit struct {
	Single bool
	Rung   int
}

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
	// Delivery is which engine carries each track (mkvgo packager, ffmpeg,
	// the client's decoder) - see DeliveryStrategy. Decided here; the adapter
	// executes it and never picks an engine on its own.
	Delivery DeliveryStrategy
	// FingerprintHash is the DecisionFingerprint.Hash() addressing the cache and
	// the ffmpeg session for this plan.
	FingerprintHash string
	// Fingerprint is the full cache-key object behind FingerprintHash, handed to
	// the producers.
	Fingerprint DecisionFingerprint
	// WasmAudio is true when the AC-3/E-AC-3 audio is served as an out-of-band
	// rendition for the browser wasm decoder (the master is video-only).
	WasmAudio bool
	// Route is THE resolution of how the session encodes (ADR-0030): codec,
	// pipeline topology, frozen backend, spill. Class, VideoCodec, HWAccel and
	// the fingerprint's HW fields are projections of it.
	Route ResolvedRoute
	// Admission is what the shaper answered for this stream (pin, granted, ceiling).
	Admission AdmissionGrant
	// Fit is the single rung the master advertises when the client's line
	// fitted the plan (LineFit); zero = every rung of Ladder.
	Fit LineFit
}

// NeedsVideoEncode reports whether serving this plan spends encode CPU/GPU (a real
// ffmpeg transcode) rather than a copy. The admission limiter only bills these.
func (p PlaybackPlan) NeedsVideoEncode() bool {
	return !p.Route.Copy
}

// UsesFFmpeg reports whether serving this plan spawns any ffmpeg process at all
// (a transcode, the ffmpeg copy remux, or the hybrid master's audio renditions)
// - i.e. it is NOT the zero-process mkvgo presentation.
func (p PlaybackPlan) UsesFFmpeg() bool {
	return p.Delivery.Video == EngineFFmpeg || p.Delivery.Audio == EngineFFmpeg
}

// PlaybackPlanResolver turns a marshalled PlaybackRequest into an immutable
// PlaybackPlan; the http adapter depends only on this interface.
type PlaybackPlanResolver interface {
	Resolve(ctx context.Context, req PlaybackRequest) (PlaybackPlan, error)
}
