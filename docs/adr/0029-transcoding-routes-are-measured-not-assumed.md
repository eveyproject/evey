# ADR-0029: Transcoding routes are measured, not assumed

- Status: Accepted

## Context

Before this decision the transcoding stack reasoned about the host, not about
the route a session actually runs. Admission priced every hardware session
from one cumulative "HW rung" benchmarked with bare presets the runtime never
emits; the accelerator was a per-host choice (VAAPI first when present); a
failed GPU session demoted the whole host on the word of one ffmpeg exit; and
the bitrate ladder was assembled from rules smeared across the plan resolver,
the producers and the ffmpeg builders.

Measured against real sessions, that model was wrong in both
directions: the cumulative rung over-priced a 4K GPU session by x1.65 and
mis-classed the CPU tone-map bridge as a pure GPU cost, so the limiter refused
viewers a box could serve and admitted sessions it could not. A box with two
accelerators (Intel VAAPI and QSV) could only use one for everything, although
HDR to SDR is only correct on the VAAPI/OpenCL path and HEVC SDR is cheaper on
QSV. And a single title whose source the GPU could not decode was enough to put
the host on CPU for every other viewer.

## Decision

Every hardware decision rests on evidence measured for the exact route it
serves, and falls back one level at a time.

1. **Admission prices the measured route.** Calibration runs each route the
   builder can emit (decode, filters, scaler, encoder, rate control, tone-map or
   bridge, production arguments) as one warm single-session encode and keeps
   the conservative sample (`RouteSample`, `internal/domain/transcoding/hw_route_cost.go`).
   `RouteCostModel` (`internal/app/transcoding/route_cost_model.go`) prices a
   session from those samples in two pools, accelerator and CPU, because a GPU
   route also spends CPU (demux, mux, audio, driver, or the whole software
   decode of a decoupled session). On a host WITH route evidence a named route
   is priced ONLY on its own sample: a (backend, encoder, route) nobody
   measured is refused by admission rather than re-priced on the H.264-shaped
   table (`chargesFor`, `video_session_limiter.go` - that re-pricing is how
   x265, hevc_vaapi, the bridge and the decoupled route were once charged as
   plain H.264), and the resolver never fields such a tuple in the first place
   (`pickBackend` drops a candidate whose route has no sample; the codec
   fallback then re-resolves HEVC onto the measured H.264 GPU route). The
   ladder-table model prices only a host with no route evidence at all (a row
   benched before evidence existed) and the unnamed, zero-route requests of
   the master-playlist shed probe; the cumulative rung is kept for diagnostics
   only. So that the refusal never bites a route the host really serves, the
   calibration measures the SAME matrix the resolver emits: every
   `RouteShape` of `RuntimeRouteShapes()` (`hw_route_cost.go`, derived from
   `EncodeRoute.CostKey` over the whole route space) that the software chain
   or a certified backend proves, and a coverage test
   (`route_evidence_plan_test.go`) fails on a shape with no measuring strategy
   unless it is listed as deliberately unmeasured with its reason (software
   AV1, an admin opt-in too slow to bench on every host).

2. **The backend is chosen per route, not per host.** Calibration certifies
   and persists every accelerator it finds (`HWBackendCap`,
   `internal/domain/transcoding/capability_backends.go`), and the resolver picks
   the backend for a request in that order: compatible with this source and
   this route, then tall enough for the rung, then cheapest by measured cost
   with a 5 % hysteresis so a recalibration does not flip the pick on noise
   (`pickBackend`, `internal/app/transcoding/playback_plan_resolver.go`).

3. **A failure falls back session, title, host - and the host only on proof.**
   The failed ffmpeg is gone (session). The title is routed to the CPU at once,
   for every viewer, by an in-memory, TTL-bounded blocklist
   (`internal/app/transcoding/hw_route_blocklist.go`), so the client's retry
   already in flight plays. The host is demoted only after a synthetic encode
   independent of the media re-fails on the same backend
   (`CalibrationService.DemoteCapability`); a route incompatibility never
   touches the host, an init failure needs two strikes in ten minutes, and a
   demotion expires after thirty minutes and is re-probed rather than
   outliving its cause (`internal/domain/transcoding/hw_failure.go`).

4. **The ladder is a pure function of persisted facts.** Source facts, the
   per-title complexity, the encode route and the delivery constraints produce
   the rungs (`internal/domain/transcoding/ladder_engine.go`); a per-title cost
   model calibrated on a measured corpus predicts what a quality costs
   (`cost_model.go`), and a per-title probe replaces the prediction only where
   the prediction is out of its margin (`title_probe.go`, `probe_trigger.go`).
   The resolver freezes the result on the session's admission pin and never
   derives it twice (ADR-0024).

Rejected: keeping one host-wide HW cost and widening its safety margin (it
was wrong by a factor, not by a margin); electing the backend once at boot
(the right backend depends on the source and the route); demoting on the
first failure with a shorter TTL (the failures observed were the media's or
the route's far more often than the GPU's); a library-wide probe sweep (a
probe is real encodes, and one scene does not beat the model on the titles it
confirms).

## Consequences

- On a host with route evidence, a named route with no measurement is REFUSED
  for lack of evidence, never re-priced on the table: the resolver spills it
  to a measured route (the H.264 variant on the same GPU, else software) and
  the calibration matrix is derived from the resolver's own shapes so that
  refusal only ever meets a route the host does not serve. A new accelerator
  (RKMPP on arm64) enters by being certified and measured through the
  same probes, not by a table entry; a new route class or codec enters by
  extending `RuntimeRouteShapes` and its measuring strategy together.
- The price is the session's STEADY-STATE consumption, a budget of averages,
  not of instantaneous peaks (`RouteCostModel`, `route_cost_model.go`): a
  software session is charged at the rate it is really paced at
  (`SoftwareReadRate`, 1.5x, one constant with the producer's default - it was
  paced at 1.5x and priced at 1.2x); a hardware session bursts unpaced until
  it is `ThrottlePauseAhead` segments ahead, is SIGSTOPped, and thereafter
  runs at the viewer's pace, so its steady state is the priced one. Several
  sessions starting at the same instant can saturate the CPU for the few
  seconds of their bursts; that transient is accepted rather than traded for
  a slower start and seek on every session, and bounded by the global session
  caps, not by this model. The CPU pool's denominator is the exact cgroup
  quota (`sysinfo.CPUCapacity`, 1.5 for `cpus: 1.5`), never the rounded-up
  worker count (`CPUBudget`). Every rate threshold of the calibration follows
  the probed frame rate of the sample it runs on (`sampleRate`), not a
  literal 24: an operator's 60p clip is judged at 60.
- The backend pick ranks routes on their pressure on the most constrained pool
  (the larger of the accelerator and CPU fractions, the figure `MaxStreams`
  caps on), not on the accelerator alone: a decoupled or bridged route slightly
  cheaper on the GPU but several cores dearer on the CPU no longer wins over
  one that is nearly free on the CPU.
- Two accelerators on one box each serve what they are best at; the admin
  sees every backend, its routes and its samples.
- A broken source costs its viewers one failed session, not the host its GPU.
  The price is a slower host verdict: the confirming probe is several
  seconds, serialised behind one mutex so concurrent failures of one title
  debounce into one decision.
- The ladder's inputs are persisted and versioned (`CapabilityProbeVersion`,
  `CostModel.Version`, `TitleProbeProtocol`): a change to any of them is a new
  cache hash, never a silent shift under existing segments.
- The measurement is route-faithful on the video side and does not yet fold
  in the audio rendition's CPU; admission under-counts that by a few
  hundredths of a core, on the safe side.
