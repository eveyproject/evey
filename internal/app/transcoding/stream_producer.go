package transcoding

import (
	"context"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// SourceDims carries the source video's shape into the pure Profiles()
// shaping: value-typed so a producer never reaches back into mutable state
// (Profiles feeds DecisionFingerprint.Hash(); drift between manifest-time and
// segment-time shaping is a permanent cache miss).
type SourceDims struct {
	Width  int
	Height int
	// Codec is the source video codec, for the HW-decode gate and the
	// decoupled session's ceiling.
	Codec string
	// TenBit marks a >=10-bit source (the 10-bit HW VPP hazard gate).
	TenBit bool
	// PixelFormat is the probed pix_fmt ("" unknown): the GPU decoder's
	// chroma/depth gate (transcoding.HWChromaDecodable).
	PixelFormat string
	// HDRRange is SourceCopyVideoRange: "PQ", "HLG" or "" (SDR, Dolby Vision).
	HDRRange string
	// Telecine: inverse telecine needed, a software-only filter.
	Telecine bool
	// Interlaced: known interlaced (flag or idet) - the QSV session-level
	// deinterlacer gate (SourceInterlaced).
	Interlaced bool
}

// facts projects the dims onto the ladder engine's SourceFacts (no bitrate:
// the producers' shape never read it).
func (d SourceDims) facts() transcoding.SourceFacts {
	return transcoding.SourceFacts{Width: d.Width, Height: d.Height, Codec: d.Codec, TenBit: d.TenBit, PixelFormat: d.PixelFormat, HDRRange: d.HDRRange, Telecine: d.Telecine}
}

// StreamProducer is the seam between the HTTP handler and the machine that
// produces HLS segments: SoftwareProducer (paced single-rung encoders, the
// client waits) or *SegmentOrchestrator (capacity-capped burst-ahead machine
// for the GPU). The ProducerSelector picks one per request.
type StreamProducer interface {
	// Profiles shapes the ABR ladder for this producer. MUST be pure in its
	// inputs: the result feeds DecisionFingerprint.Hash(), so manifest-time and
	// segment-time must yield the same ladder or the cache misses forever.
	Profiles(base []transcoding.HLSProfile, src SourceDims, decision transcoding.PlaybackDecision, capacity transcoding.EncodeCapability) []transcoding.HLSProfile

	// EnsureSegmentRange guarantees the segment will be produced (or is already
	// on disk). Returns immediately; callers block on cache.Wait. The software
	// producer NEVER returns ErrCapacityExceeded.
	EnsureSegmentRange(ctx context.Context, fp DecisionFingerprint, variantIdx, segmentIdx int) error

	// PreWarm starts producing segments for fp in the background (fire-and-
	// forget). variantIdx matters in SingleVariant mode, where each rung has its
	// own ffmpeg.
	PreWarm(fp DecisionFingerprint, variantIdx, startSegIdx int)

	// WaitBudget is how long the segment handler blocks on cache.Wait for this
	// producer (HW ~25 s; software long, the client waits).
	WaitBudget() time.Duration

	// NotifyClientHint forwards CMCD telemetry so the producer can adjust
	// pacing; it never changes which variant is served. variantIdx is a FILTER:
	// a hint about rung 0 must not touch the rung the player just abandoned.
	NotifyClientHint(mediaItemID string, variantIdx, bufferLengthMs int, starved bool, segmentIdx int)

	// ActiveSessionCount reports how many ffmpeg sessions are live.
	ActiveSessionCount() int

	// RemoveSession kills every session producing for the given media ID.
	RemoveSession(mediaItemID string)

	// Close tears down every session and frees their disk segments.
	Close()
}

// ManifestProvider is the optional capability a StreamProducer exposes when it
// owns its HLS media playlist (the copy rung's keyframe-aligned plan). The
// assertion matches any *SoftwareProducer, so the handler's copy gate scopes it.
type ManifestProvider interface {
	Manifest(ctx context.Context, fp DecisionFingerprint) ([]byte, error)
}

var _ ManifestProvider = (*SoftwareProducer)(nil)

// ProducerSelector holds the software and hardware producers; the resolved
// plan's route says which one serves a request (PlanResolver.ProducerForPlan
// -> forRoute). No gate is evaluated here (ADR-0030).
type ProducerSelector struct {
	software StreamProducer
	hardware StreamProducer

	// playback counts reads served WITHOUT an ffmpeg session (mkvgo, direct
	// play), which both producers are blind to; every anti-starvation guard
	// asks this one object whether someone is watching. nil = zero.
	playback *PlaybackActivity
}

// WithPlaybackActivity wires the non-ffmpeg playback tracker (chainable).
func (s *ProducerSelector) WithPlaybackActivity(a *PlaybackActivity) *ProducerSelector {
	s.playback = a
	return s
}

// NewProducerSelector wires the two producers. Either producer may be nil
// only in degenerate test setups.
func NewProducerSelector(software, hardware StreamProducer) *ProducerSelector {
	return &ProducerSelector{software: software, hardware: hardware}
}

// forRoute returns the producer of the resolved route's side: the hardware
// one when the accelerator encodes and one is wired, else the software one.
// No gate is evaluated here - the route already decided (ADR-0030).
func (s *ProducerSelector) forRoute(hw bool) StreamProducer {
	if hw && s.hardware != nil {
		return s.hardware
	}
	return s.software
}

// Software returns the software producer, for the admission spill (GPU pool
// full): the producer the pinned fingerprint describes.
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

// ActiveSessionCount sums live FFMPEG sessions across both producers: the
// "how much encoding work is running" number (admin dashboard, gauge). A
// direct play or an mkvgo copy is not a transcode and is not counted here.
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
// EVERY serving path (ffmpeg sessions plus mkvgo / direct play): the
// anti-starvation signal background workers yield on.
func (s *ProducerSelector) ActivePlaybackCount() int {
	return s.ActiveSessionCount() + s.playback.ActiveSessionCount()
}

// RemoveSession fans the kill out to both producers (a media item could have
// live sessions on either path, e.g. an admin toggled HW mid-stream).
func (s *ProducerSelector) RemoveSession(mediaItemID string) {
	if s.software != nil {
		s.software.RemoveSession(mediaItemID)
	}
	if s.hardware != nil {
		s.hardware.RemoveSession(mediaItemID)
	}
}
