// Package transcoding is the domain of how a title is served: the route a
// session runs, the ladder it encodes, what it costs, what the host proved it
// can do, and how a failure is read. Three natures live here and are worth
// telling apart when reading:
//
//   - the MODEL - what a session is:
//     route.go (Topology, the seven legal pipeline shapes, RouteShape, the
//     builder's RouteExpressibility port), ladder_engine.go (SourceFacts,
//     EncodeRoute, LadderInput, the LadderBuilder), ladders.go (the content
//     class tables and the ladder caps), model.go / codec.go / playback_decision.go;
//   - the EVIDENCE - what this host measured:
//     capability.go and capability_backends.go (EncodeCapability, the per-
//     backend proofs, CapabilityProbeVersion), hw_route_cost.go (RouteSample,
//     RouteCostKey: the per-route cost evidence of ADR-0029), cost_model.go and
//     complexity.go (the per-title cost model and its inputs), title_probe.go;
//   - the POLICY - what the operator and the product decided:
//     derive.go (DerivePolicy: margins, floors, how evidence becomes a
//     ceiling), hw_mode.go (the hardware mode), caps_override.go, probe_trigger.go,
//     hw_failure.go (FailureClass: how a failed session is read and how far
//     its verdict reaches - session, title, host).
//
// Suggested reading order for the decision path: route.go -> ladder_engine.go
// -> capability.go -> derive.go -> hw_failure.go. The application layer
// (internal/app/transcoding) resolves a PlaybackPlan from these once per
// request (ADR-0024, ADR-0030); the ffmpeg adapter executes it.
package transcoding
