package transcoding

// resolver_producer.go - the producer a plan spawns on, a projection of
// plan.Route.

import (
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ProducerFor picks the producer for an already-resolved decision, so the
// producer chosen for a segment matches the one the ladder was shaped for
// (and thus the cache hash). Falls back to the orchestrator when no selector
// is wired (hand-built test handlers); returns nil only when neither is set.
func (pr *PlanResolver) ProducerFor(req PlaybackRequest, decision transcoding.PlaybackDecision, isCopy bool) StreamProducer {
	return pr.producerForRoute(pr.routeWithPin(req, decision, isCopy))
}

// ProducerForPlan picks the producer a resolved plan spawns on: the copy
// producer for the remux rung, else the producer of the route's side - a
// PROJECTION of plan.Route, which already carries the spill and the block.
func (pr *PlanResolver) ProducerForPlan(plan PlaybackPlan) StreamProducer {
	return pr.producerForRoute(plan.Route)
}

func (pr *PlanResolver) producerForRoute(route ResolvedRoute) StreamProducer {
	if pr.Selector != nil {
		if route.Copy {
			return pr.CopyProducer
		}
		return pr.Selector.forRoute(route.HW())
	}
	return pr.Orchestrator
}

// audioEncoderID is the nil-safe read of the injected probe: an unwired
// resolver keeps the native encoder, which is exactly the pre-feature
// behaviour, bytes and hash alike.
func (pr *PlanResolver) audioEncoderID() string {
	if pr.AudioEncoderID == nil {
		return ""
	}
	return pr.AudioEncoderID()
}
