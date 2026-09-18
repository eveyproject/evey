package transcoding

// resolver_producer.go - the producer a plan spawns on, a projection of
// plan.Route.

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
