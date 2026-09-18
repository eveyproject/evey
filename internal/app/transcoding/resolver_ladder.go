package transcoding

// resolver_ladder.go - the ladder shaped for the resolved route and the
// admission grant on it (the pin, the ceiling, the spill).

import (
	"context"
	"fmt"
	"slices"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// pinKeyFor identifies a stream for the admission shaper. Coarser than the
// decision-fingerprint hash on purpose: burn-in/audio variants of the same
// title share one pinned ceiling (their video ladder is the same shape), while
// distinct titles or codecs never collide. It must be derivable identically at
// manifest- and segment-time, so it reads the route BEFORE the pool spill
// (the spill is the pin's own answer, never its input): the codec the route
// carries and, for a title whose GPU route just failed, the block itself - the
// ladder frozen for its GPU session must not be served to its CPU session.
//
// It also carries every REQUEST-scoped ladder input (screen height, bitrate
// ceiling): the frozen ladder is read back by key, so two viewers of one title
// with different caps must not share a pin or the second inherits the first's.
func (pr *PlanResolver) pinKeyFor(req PlaybackRequest, decision transcoding.PlaybackDecision, route ResolvedRoute) string {
	key := fmt.Sprintf("%s|%s|%s|sh=%d|mh=%d|bw=%d|lvl=%d", req.MediaID, route.Codec, decision.VideoDecision,
		req.ScreenHeight, req.Client.MaxHeight, pr.videoBitrateCeilingKbps(req, decision), req.Client.MaxH264Level)
	if req.Client.MaxH264Height > 0 {
		key += fmt.Sprintf("|h264h=%d", req.Client.MaxH264Height) // shapes an H.264 ladder: its own pin
	}
	if route.Spill == SpillTitleBlocked {
		key += "|rb=1"
	}
	return key
}

// shapeLadderFor shapes the ladder for the resolved route and runs admission
// on it. Returns the ladder, the grant, and whether admission spilled the
// stream to the CPU route (the caller rewrites the route; the ladder returned
// is already the CPU shape).
func (pr *PlanResolver) shapeLadderFor(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, route ResolvedRoute, pinKey string) (profiles []transcoding.HLSProfile, grant AdmissionGrant, spilled bool) {
	// Read-verbatim (Étape 2): a live stream's ABR ladder is shaped ONCE and
	// frozen on its admission pin; return it unchanged so the manifest and every
	// segment shape the SAME ladder BY CONSTRUCTION - immune to a mid-stream drift
	// of the mutable inputs below (complexity class, capability, CMCD/av1 caps)
	// that would otherwise desync a player's URLs from the cached bytes. Copy
	// rungs take no admission pin (never frozen); their single source-res rung is
	// stable regardless. streamKey is set only on the freezable (transcode) path,
	// and gates the freeze at the tail.
	isCopy := route.Copy
	var streamKey string
	if !isCopy && pr.AdmissionFrozenLadder != nil {
		streamKey = pinKey
		if frozen, ok := pr.AdmissionFrozenLadder(streamKey); ok {
			// The pin governs: a stream pinned to the CPU while the GPU pool was
			// full stays there for its whole life (the URLs address CPU bytes).
			if route.HW() && pr.AdmissionPinnedClass != nil {
				if cls, ok := pr.AdmissionPinnedClass(streamKey); ok && cls != LadderClassHW {
					spilled = true
				}
			}
			return frozen, AdmissionGrant{PinKey: streamKey, Granted: true}, spilled
		}
	}

	complexity := pr.complexityOf(ctx, req.MediaID)
	pol := pr.policy(req)
	in := transcoding.LadderInput{
		Source:     pr.sourceFacts(req),
		Complexity: complexity,
		Route:      route.Encode(),
		Delivery:   pr.deliveryConstraints(pol, req, decision),
	}
	laddered := pr.ladderBuilder().Build(in)

	if pr.AdmissionRoute != nil && len(laddered) > 0 && !isCopy {
		// The class admission prices: the route's - a blocked title IS a
		// software stream now (its route was spilled), so admission prices the
		// CPU pool and never offers the GPU as a fallback.
		natural := route.Class()
		// The spill ladder (GPU pool full, CPU pool free) is the same request
		// shaped for the CPU route - every delivery cap included. It used to be
		// the bare producer shape, so a spilled viewer lost the screen, level and
		// bitrate caps the master had negotiated.
		spillRoute := route.spill(SpillPoolFull, decision)
		softwareShape := func() []transcoding.HLSProfile {
			if pr.Selector == nil || pr.Selector.Software() == nil {
				return nil
			}
			sw := in
			sw.Route = spillRoute.Encode()
			return pr.ladderBuilder().Build(sw)
		}
		var fallback func() []int
		if natural == LadderClassHW && pr.Selector != nil {
			fallback = func() []int { return profileHeights(softwareShape()) }
		}
		snap := snapshot(req)
		key := pinKey
		g := pr.AdmissionRoute(key, RouteAdmission{
			Natural:         natural,
			Heights:         profileHeights(laddered),
			Src:             snap.Class(),
			Route:           route.CostKey(),
			SourceFPS:       snap.FPS,
			FallbackHeights: fallback,
			FallbackRoute:   spillRoute.CostKey(),
		})
		grant = AdmissionGrant{PinKey: key, Granted: g.OK, Ceiling: g.Ceiling}
		if g.OK {
			if g.Class != natural && g.Class == LadderClassSoftware {
				if swLadder := softwareShape(); len(swLadder) > 0 {
					laddered = swLadder
				}
				spilled = natural == LadderClassHW
			}
			if g.Ceiling > 0 {
				laddered = transcoding.CapLadderToHeight(laddered, g.Ceiling)
			}
		}
	}
	// Freeze this shaped ladder on the admission pin (created by AdmissionRoute
	// just above) so every later manifest-/segment-time call reads it verbatim.
	// No-op when no pin was created (nothing fit ⇒ refused, or admission unwired):
	// FreezeLadder finds no pin and does nothing, and the next call re-derives.
	if streamKey != "" && pr.AdmissionFreezeLadder != nil {
		pr.AdmissionFreezeLadder(streamKey, laddered)
	}
	return laddered, grant, spilled
}

// profileHeights lists the heights of a shaped ladder, in order.
// videoBitrateCeilingKbps is the tightest of the request cap, the profile cap,
// a bitrate_exceeded decision and the server setting; 0 = follow the source.
func (pr *PlanResolver) videoBitrateCeilingKbps(req PlaybackRequest, decision transcoding.PlaybackDecision) int64 {
	var ceiling int64
	tighten := func(v int64) {
		if v > 0 && (ceiling == 0 || v < ceiling) {
			ceiling = v
		}
	}
	tighten(int64(req.MaxBitrateKbps))
	tighten(int64(req.Prefs.EffectiveMaxBitrateKbps()))
	if slices.Contains(decision.TranscodeReasons, "bitrate_exceeded") {
		tighten(decision.VideoBitrate)
	}
	if pr.MaxVideoBitrateKbps != nil {
		tighten(int64(pr.MaxVideoBitrateKbps()))
	}
	return ceiling
}

func profileHeights(profiles []transcoding.HLSProfile) []int {
	out := make([]int, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Height)
	}
	return out
}

func (pr *PlanResolver) ladderBuilder() transcoding.LadderBuilder {
	if pr.Ladder != nil {
		return pr.Ladder
	}
	return transcoding.StaticLadderBuilder{}
}

// complexityOf reads the title's persisted complexity (class + metrics, the
// cost model's input), through the short-TTL cache on the segment hot path.
// An unclassified title resolves to the film_normal class and no metrics.
func (pr *PlanResolver) complexityOf(ctx context.Context, mediaID string) transcoding.ComplexityVector {
	entry := transcoding.ComplexityEntry{Class: transcoding.ClassFilmNormal}
	if pr.ComplexityRepo == nil || mediaID == "" {
		return transcoding.ComplexityVector{Class: entry.Class, Metrics: entry.Metrics}
	}
	if pr.ClassLookup != nil {
		if e, ok := pr.ClassLookup.Get(mediaID); ok {
			return transcoding.ComplexityVector{Class: e.Class, Metrics: e.Metrics}
		}
	}
	if e, found, err := pr.ComplexityRepo.Get(ctx, mediaID); err == nil && found {
		if e.Class.IsValid() {
			entry.Class = e.Class
		}
		entry.Metrics = e.Metrics
	}
	if pr.ClassLookup != nil {
		// Cache the RESOLVED entry (default included) so an unclassified
		// title doesn't re-query on every segment either.
		pr.ClassLookup.Set(mediaID, entry, pr.ClassLookupTTL)
	}
	return transcoding.ComplexityVector{Class: entry.Class, Metrics: entry.Metrics}
}

// QualityPolicy is the rate-control policy the encoder plans read.
func (pr *PlanResolver) QualityPolicy() transcoding.QualityPolicy {
	if pr.Quality != nil {
		return pr.Quality
	}
	return transcoding.StaticQualityPolicy{}
}

// sourceFacts projects the request's streams onto the ladder engine's input.
func (pr *PlanResolver) sourceFacts(req PlaybackRequest) transcoding.SourceFacts {
	return snapshot(req).Facts()
}

// deliveryConstraints gathers every cap the request, the operator, the QoE
// feedback and the host put on the ladder.
func (pr *PlanResolver) deliveryConstraints(pol HWPolicy, req PlaybackRequest, decision transcoding.PlaybackDecision) transcoding.DeliveryConstraints {
	dc := transcoding.DeliveryConstraints{
		ScreenHeight:            req.ScreenHeight,
		MaxHeight:               req.Client.MaxHeight,
		MaxH264Level:            req.Client.MaxH264Level,
		MaxH264Height:           req.Client.MaxH264Height,
		VideoBitrateCeilingKbps: pr.videoBitrateCeilingKbps(req, decision),
		Capability:              pr.capabilityOrFallback().EncodeCapability,
	}
	if pr.CapOverride != nil {
		if ov, ok := pr.CapOverride(snapshot(req).Codec, snapshot(req).Height); ok {
			dc.QoEMaxHeight = ov.MaxHeight
		}
	}
	dc.AV1MaxHeight = pol.AV1MaxHeight
	return dc
}
