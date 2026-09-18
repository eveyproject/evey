package transcoding

// resolver_plan_test.go - how a test holding a DECISION reaches the plan:
// planFor is Resolve past Decide (resolveDecided), with the source snapshot
// and the policy taken exactly as Resolve takes them. Every facet a test
// reads (codec, backend, route, ladder, class, fingerprint, producer, pin)
// is a projection of that plan; nothing here derives one on its own.

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// planFor resolves the plan for req under decision, as Resolve would past
// its own Decide.
func (pr *PlanResolver) planFor(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision) PlaybackPlan {
	req.Source = SnapshotOf(req)
	req.Policy = pr.hwPolicy()
	return pr.resolveDecided(ctx, req, decision)
}

// routeOf reads a plan's route the way the old EffectiveVideoCodec answered:
// the codec the segments carry, the backend, its device and features,
// whether the accelerator encodes and whether it software-decodes.
func routeOf(plan PlaybackPlan) (codec, accel, device string, features []string, usable, softwareDecode bool) {
	r := plan.Route
	return r.Codec, r.Backend.Name, r.Backend.Device, r.Backend.Features, r.HW(), r.Topology.SoftwareDecode()
}

// streamsOf is the inverse of SourceDimsOf for the facts a test names on a
// SourceDims literal: a request built from it snapshots back to the same
// dims (codec, size, depth, format, HDR range, telecine, interlacing).
func streamsOf(d SourceDims) []*library.MediaStream {
	s := &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: d.Codec}
	if d.Width > 0 {
		s.Width = new(d.Width)
	}
	if d.Height > 0 {
		s.Height = new(d.Height)
	}
	if d.TenBit {
		s.BitDepth = new(10)
	}
	if d.PixelFormat != "" {
		s.PixelFormat = new(d.PixelFormat)
	}
	switch d.HDRRange {
	case "PQ":
		s.ColorTransfer, s.ColorSpace = new("smpte2084"), new("bt2020nc")
	case "HLG":
		s.ColorTransfer, s.ColorSpace = new("arib-std-b67"), new("bt2020nc")
	}
	if d.Telecine {
		s.IdetTelecine = new(true)
	}
	if d.Interlaced {
		s.IdetInterlaced = new(true)
	}
	return []*library.MediaStream{s}
}

// routeUsable is the old HWRouteUsable gate read off the plan: does the
// accelerator encode this decision for this source?
func (pr *PlanResolver) routeUsable(decision transcoding.PlaybackDecision, src SourceDims) bool {
	return pr.planFor(context.Background(), PlaybackRequest{Streams: streamsOf(src)}, decision).Route.HW()
}

// naturalRouteOf is the route before the per-title block and the pool
// spill: what the host would serve this request with, all else being free.
// The golden reads it beside the plan's route to show what the block and
// admission did.
func (pr *PlanResolver) naturalRouteOf(req PlaybackRequest, decision transcoding.PlaybackDecision) ResolvedRoute {
	req.Source = SnapshotOf(req)
	req.Policy = pr.hwPolicy()
	return pr.naturalRoute(req, decision, pr.IsCopy(decision, req.Streams, req.SourceBytes))
}

// livePin is a stub of the admission pin as the pipeline meets it: a pin
// that exists comes with the ladder frozen under it (GrantRoute then
// FreezeLadder, in production), so a pinned class is read by the shaping
// ONLY beside a frozen ladder. ok=false is no pin at all.
func livePin(class LadderClass, ok bool) (pinned func(string) (LadderClass, bool), frozen func(string) ([]transcoding.HLSProfile, bool)) {
	pinned = func(string) (LadderClass, bool) { return class, ok }
	frozen = func(string) ([]transcoding.HLSProfile, bool) {
		if !ok {
			return nil, false
		}
		return []transcoding.HLSProfile{{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 192}}, true
	}
	return pinned, frozen
}
