package transcoding

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// SourceDims carries the source video stream's resolution and HDR-relevant
// shape into the pure Profiles() shaping. Kept tiny and value-typed so a
// producer's Profiles() can stay deterministic - it never reaches back into
// mutable handler state, the whole point of the cache-hash invariant
// (segment_cache.go: Profiles is part of DecisionFingerprint.Hash(), so any
// drift between manifest-time and segment-time shaping is a permanent miss).
type SourceDims struct {
	Width  int
	Height int
	// Codec is the source video stream's codec ("h264"/"hevc"/…). The selector
	// needs it for the HW-decode gate (a pre-Skylake iGPU decodes H.264 but not
	// HEVC), and the hardware producer caps a DECOUPLED session's ladder from
	// it (see SegmentOrchestrator.Profiles).
	Codec string
	// TenBit marks a >=10-bit source (HEVC Main10 / yuv420p10le). Producers
	// don't currently branch on it, but it travels alongside Width/Height so a
	// producer can refine its ceiling without a signature change. The selector
	// uses it for the 10-bit HW VPP hazard gate.
	TenBit bool
	// HDRRange is SourceCopyVideoRange: "PQ", "HLG" or "" (SDR, Dolby Vision).
	HDRRange string
	// Telecine: inverse telecine needed, a software-only filter.
	Telecine bool
}

// StreamProducer is the seam between the HTTP handler and the machine that
// actually produces HLS segments. Two implementations exist:
//
//   - SoftwareProducer: the paced model - ONE long-lived, input-paced
//     (`-readrate`) ffmpeg per DecisionHash, single-rendition, restart-on-seek,
//     no capacity rejection (the client waits), idle GC. The sane software
//     path for a weak NAS with hw_accel off.
//   - *SegmentOrchestrator: the multi-session, capacity-capped, burst-ahead
//     machine built for the hardware path (GPU parallelises rungs cheaply).
//
// The ProducerSelector picks one per request. Both satisfy this interface so
// the handler dispatch sites are producer-agnostic.
type StreamProducer interface {
	// Profiles shapes the ABR ladder for this producer. MUST be pure and
	// deterministic in its inputs: the result feeds DecisionFingerprint.Hash()
	// (via ProfileFingerprint), so the SAME (base, src, decision, capacity)
	// must yield the SAME ladder at manifest-time and at segment-time or the
	// segment cache misses forever.
	//
	// capacity is passed as an immutable snapshot argument - the producer reads
	// no mutable state. The HW producer returns the full ladder capped at the
	// HW ceiling; the software producer collapses to ONE sustainable rung.
	Profiles(base []transcoding.HLSProfile, src SourceDims, decision transcoding.PlaybackDecision, capacity transcoding.EncodeCapability) []transcoding.HLSProfile

	// EnsureSegmentRange guarantees the segment will be produced (or is already
	// on disk). Returns immediately; callers block on cache.Wait. The software
	// producer NEVER returns ErrCapacityExceeded.
	EnsureSegmentRange(ctx context.Context, fp DecisionFingerprint, variantIdx, segmentIdx int) error

	// PreWarm starts producing segments for fp in the background (cold-start
	// optimisation). Fire-and-forget, bounded by the producer's own lifecycle.
	// variantIdx is the rung the caller is about to serve: in SingleVariant mode
	// each rung has its own ffmpeg, so pre-warming the wrong one wastes a whole
	// process the player will never consume. In multi-variant mode variantIdx is
	// forwarded to EnsureSegmentRange but ignored for session-matching, same as
	// everywhere else in that path.
	PreWarm(fp DecisionFingerprint, variantIdx, startSegIdx int)

	// WaitBudget is how long the segment handler should block on cache.Wait for
	// a segment from this producer. HW: ~25s. Software: long (the single paced
	// encoder is slower to land segment 0 and the client is expected to wait).
	WaitBudget() time.Duration

	// NotifyClientHint forwards CMCD telemetry (buffer length, starvation,
	// cursor) so the producer can adjust pacing. NEVER changes which variant is
	// served - the player owns rendition selection.
	//
	// variantIdx is the rung the hint describes. It is a FILTER, not a
	// selector: a hint about rung 0 must not resume, un-pause or push the
	// cursor of the rung the player just abandoned (both rungs share the
	// mediaID, and after a level switch they even share startSeg, so matching
	// on mediaID alone picked one at map-iteration random).
	NotifyClientHint(mediaItemID string, variantIdx, bufferLengthMs int, starved bool, segmentIdx int)

	// ActiveSessionCount reports how many ffmpeg sessions are live (admin
	// metrics + trickplay scheduler contract).
	ActiveSessionCount() int

	// RemoveSession kills every session producing for the given media ID
	// (admin kill-session / playback-stop). No-op when none match.
	RemoveSession(mediaItemID string)

	// Close tears down every session and frees their disk segments.
	Close()
}

// ManifestProvider is the optional capability a StreamProducer exposes when it
// owns its HLS media playlist instead of deferring to the precalculated VOD
// generator - the copy/remux rung, whose playlist is the keyframe-aligned plan.
// serveVariant type-asserts the selected producer to it, GATED on isCopyRequest:
// the assertion matches any *SoftwareProducer (incl. the transcode instance,
// copyPlanner nil), so the handler's copy gate is what actually scopes it.
type ManifestProvider interface {
	Manifest(ctx context.Context, fp DecisionFingerprint) ([]byte, error)
}

var _ ManifestProvider = (*SoftwareProducer)(nil)

// ProducerSelector routes each request to the software or hardware producer.
// The decision is two-step, in this exact order:
//
//  1. Is hardware acceleration enabled AT ALL? (the hw_accel_enabled flag /
//     hw_mode). No  -> software, unconditionally.
//  2. If enabled, is HW usable for THIS content? (the full resolveHW chain:
//     the backend encodes the chosen codec, can decode the source codec, and
//     the 10-bit / tonemap hazards are cleared). No -> software.
//
// Yes to both -> hardware. This is exactly resolveHW (the handler's
// hwUsableForContent), reused verbatim so the manifest, the fingerprint, and
// the selector never disagree on which path a request takes.
type ProducerSelector struct {
	software StreamProducer
	hardware StreamProducer

	// hwUsableForContent is the handler's resolveHW projection: it returns true
	// only when HW is enabled AND exploitable for this codec/bit-depth/tonemap
	// combination. The selector treats it as the single source of truth so the
	// cache fingerprint (which keys HW vs SW segments interchangeably) and the
	// routing stay consistent.
	hwUsableForContent func(decision transcoding.PlaybackDecision, src SourceDims) bool

	// playback counts reads served WITHOUT an ffmpeg session (the mkvgo
	// on-demand packager, direct play), which are invisible to both producers.
	// Folded in here rather than at each call site because every
	// anti-starvation guard in the server asks this one object whether someone
	// is watching - wiring it anywhere else would fix some guards and leave the
	// rest blind. nil = not wired (reads as zero).
	playback *PlaybackActivity
}

// WithPlaybackActivity wires the non-ffmpeg playback tracker, which
// ActivePlaybackCount folds in. Chainable; leaving it unset makes
// ActivePlaybackCount equal ActiveSessionCount (the prior behaviour).
func (s *ProducerSelector) WithPlaybackActivity(a *PlaybackActivity) *ProducerSelector {
	s.playback = a
	return s
}

// NewProducerSelector wires the two producers and the usability predicate.
// Either producer may be nil only in degenerate test setups; the handler nil-
// guards before route registration.
func NewProducerSelector(
	software, hardware StreamProducer,
	hwUsableForContent func(decision transcoding.PlaybackDecision, src SourceDims) bool,
) *ProducerSelector {
	return &ProducerSelector{
		software:           software,
		hardware:           hardware,
		hwUsableForContent: hwUsableForContent,
	}
}

// For returns the producer that should serve this request. Software is the
// default; hardware only when hwUsableForContent confirms the request takes
// the GPU path end-to-end.
func (s *ProducerSelector) For(decision transcoding.PlaybackDecision, src SourceDims) StreamProducer {
	if s.hardware != nil && s.hwUsableForContent != nil && s.hwUsableForContent(decision, src) {
		return s.hardware
	}
	return s.software
}

// Software returns the software producer, for callers that must FORCE the
// software route regardless of the hw gate - the admission spill (GPU pool
// full, CPU has room) pins a stream to software and needs the producer the
// pinned fingerprint describes.
func (s *ProducerSelector) Software() StreamProducer {
	return s.software
}

// Close tears down both producers. Safe to call once at shutdown.
func (s *ProducerSelector) Close() {
	if s.software != nil {
		s.software.Close()
	}
	if s.hardware != nil {
		s.hardware.Close()
	}
}

// ActiveSessionCount sums live FFMPEG sessions across both producers. This is
// the "how much encoding work is running" number: it feeds the admin
// dashboard's ActiveTranscodes and the evey_active_hls_sessions gauge, so it
// must stay ffmpeg-only. A direct-play read is not a transcode, and neither is
// an mkvgo copy - counting them here would inflate both readouts.
//
// For "is anyone reading the disk right now", which is a different and wider
// question, use ActivePlaybackCount.
func (s *ProducerSelector) ActiveSessionCount() int {
	n := 0
	if s.software != nil {
		n += s.software.ActiveSessionCount()
	}
	if s.hardware != nil {
		n += s.hardware.ActiveSessionCount()
	}
	return n
}

// ActivePlaybackCount reports how many media are being read right now across
// EVERY serving path - ffmpeg sessions plus the paths that spawn none (the
// mkvgo on-demand packager, direct play).
//
// This is the anti-starvation signal: every background worker that yields under
// live playback asks this question, and the answer must not depend on which
// machinery happens to serve the viewer. A copy-eligible source is served
// entirely by mkvgo and a direct-play by a bare ServeFile, so for those the
// ffmpeg terms are both zero and the non-ffmpeg term is the ONLY evidence that
// anyone is watching.
func (s *ProducerSelector) ActivePlaybackCount() int {
	return s.ActiveSessionCount() + s.playback.ActiveSessionCount()
}

// RemoveSession fans the kill out to both producers - a media item could have
// live sessions on either path (e.g. an admin toggled HW mid-stream). No-op on
// the producer with no matching session.
func (s *ProducerSelector) RemoveSession(mediaItemID string) {
	if s.software != nil {
		s.software.RemoveSession(mediaItemID)
	}
	if s.hardware != nil {
		s.hardware.RemoveSession(mediaItemID)
	}
}
