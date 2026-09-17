# ADR-0029: Transcoding routes are measured, not assumed

- Status: Accepted
- Date: 2026-09-13 (decisions taken 2026-09-12/13, recorded 2026-09-17)

## Context

Until September 2026 the transcoding stack reasoned about the host, not about
the route a session actually runs. Admission priced every hardware session
from one cumulative "HW rung" benchmarked with bare presets the runtime never
emits; the accelerator was a per-host choice (VAAPI first when present); a
failed GPU session demoted the whole host on the word of one ffmpeg exit; and
the bitrate ladder was assembled from rules smeared across the plan resolver,
the producers and the ffmpeg builders.

Measured on 2026-09-12/13 against real sessions, that model was wrong in both
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
   decode of a decoupled session). The ladder-table model stays as the fallback
   for a route with no evidence; the cumulative rung is kept for diagnostics
   only.

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

- A route with no measurement is priced the old way and never refused for
  lack of evidence; a new accelerator (RKMPP, arm64, 2026-09) enters by being
  certified and measured through the same probes, not by a table entry.
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
