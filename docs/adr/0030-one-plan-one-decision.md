# ADR-0030: One plan, one decision

- Status: Accepted
- Date: 2026-09-17

## Context

ADR-0024 made the HLS read path resolve its plan once per request in the app
layer. Below that resolution the ROUTE - the codec the segments carry, the
accelerator, whether the CPU decodes ahead of the GPU, where the HDR tone-map
runs, whether the HDR is kept - was still derived four times: once to shape
the ladder (`routeFor`), once to build the cache fingerprint (`Fingerprint`,
which re-applied the admission spill, the per-title block and the codec
fallback by hand), once to bill admission (`LadderClassFor` on the request,
`LadderClassOf` on the fingerprint) and once to pick the producer
(`spilledToSoftware`). The ffmpeg builder then re-decided on its own
(`hwUsable`: a burn-in without a GPU compositor, a decoupled session without
an upload chain, a tone-map without a GPU filter) and fell back to a software
encode silently, so the app mirrored those rules to stay in step.

The same object wore five vocabularies: `EncodeRoute`, the `HW*` fields of
`DecisionFingerprint`, `LadderClass`, `RouteCostClass`/`RouteShape`, and the
calibration proofs `HWRoute*`. Coherence rested on equivalence tests
("X agrees with Y") and some sixty "mirrors / must agree / keep in sync"
comments. Every new route or codec had to be declared in six places; the
calibration matrix and the resolver drifted apart until the matrix was
derived from the resolver's shapes (ADR-0029, 2026-09-17).

## Decision

After `PlanResolver.Resolve` returns, no layer decides anything about codec,
backend, pipeline, tone-map, ladder or admission. Every layer below PROJECTS
or EXECUTES the plan.

1. **`SourceSnapshot`** - the source facts are read from the streams once, at
   the start of `Resolve`, with the existing `Source*` helpers, and every
   later consumer reads the snapshot (`SourceDims`, `SourceClass` become
   views of it).

2. **`Topology`** replaces the boolean soup of `EncodeRoute`: an explicit
   pipeline `{Decode: CPU|GPU, ToneMap: None|GPU|CPU, Encode: CPU|GPU,
   Output: SDR|HDRPassthrough}` built by a constructor that refuses illegal
   states. The route cost class, the calibration matrix
   (`RuntimeRouteShapes`) and the builder's expressibility check are all
   functions of the topology. GPU deinterlacing and GPU subtitle compositing
   are NOT topology: they are per-request runtime options carried by the
   route (`Deinterlace`, `Overlay`), because they change neither the pools,
   the cost class nor the calibration shape - only the filtergraph.

3. **`ResolvedRoute`** - computed once by `Resolve`, carried by
   `PlaybackPlan`: `{Codec, Topology, Backend: BackendSelection{Name,
   Device, Features}, Spill: SpillReason, Deinterlace, Overlay}`. The
   backend is a FROZEN selection (what the builder needs), not the
   capability row. `Route.Class()` and `Route.CostKey()` are the only
   DERIVATIONS of the billed class and the route key. `LadderClassOf(fp)` and
   `RouteCostKeyOf(fp)` stay as pure projections of the fingerprint - the
   producers hold only the fingerprint at spawn - and are locked equal to the
   route's (invariant 2); the request-side re-derivations (`encodeRoute`,
   `routeFor`, `spillRoute`, `softwareClassFor`, `spilledToSoftware`) are
   removed, and `LadderClassFor` survives only as "the natural route's class"
   for the adapter's wrappers and tests.

3. **`AdmissionGrant`** - the admission outcome is an explicit value on the
   plan `{PinKey, Granted, Ceiling, Probe}`. It carries nothing derivable from
   the route: the billed class is `Route.Class()`, the spill is
   `Route.Spill != ""` (the route was rewritten for the CPU when the grant
   spilled it). `Fingerprint` and `ProducerFor` no longer ask the shaper.

5. **Three projections** - `EncodeSpec` (the cache identity: exactly the
   fields `Hash()` serialises today), `RuntimeOptions` (what the builder
   receives and `Hash()` already excludes: device, features, GPU tone-map,
   software decode, overlay, deinterlace, transfer, quality targets) and the
   admission inputs (from the snapshot). `DecisionFingerprint` survives as
   the persisted, tested struct, assembled by `FingerprintOf(plan)`; its
   `Hash()` is byte-identical.

6. **The builder validates, never falls back** - `RouteExpressibility` is a
   domain port implemented by the ffmpeg adapter from its current gates; the
   resolver consults it when it picks a backend, so an inexpressible route is
   never resolved. `BuildFFmpegArgs` keeps the check and returns
   `ErrRouteNotExpressible` instead of switching to a software encode. That
   error is a planner/builder INVARIANT VIOLATION - a bug to log and count -
   never a reason to block the title's GPU route or demote the host.

7. **Route, cost, failure** stay three axes sharing one key: a `RouteSample`
   is indexed by `Route.CostKey()`; a hardware failure is reported with the
   `ResolvedRoute` that failed, so a demotion targets the proof of that
   topology on that backend, not "the codec".

8. **Policy read once** - the operator's hardware toggles are read into an
   `HWPolicy` at the start of `Resolve`, not at eleven sites along the way.

Rejected: splitting the persisted `EncodeCapability` into Evidence /
Capability / Policy (compatibility of the persisted row and its probe
version for a naming gain); freezing the route on the admission pin (a
behaviour change - the route can still move between manifest and segment
when the capability changes; the hash forks by backend so the cache stays
safe - to be decided on its own); pacing the hardware orchestrator (see
ADR-0029's steady-state pricing).

## Invariants (each one is a test)

1. One derivation: outside the route resolution no code calls
   `EffectiveVideoCodec`, `pickBackend` or the `*Wanted` gates.
2. Total projection: `FingerprintOf(plan)` is a pure function of the plan;
   the class and the route key read off the fingerprint equal
   `plan.Route.Class()` and `plan.Route.CostKey()` for every valid topology.
3. Legal topologies only: the constructor refuses the rest; the calibration
   matrix, the cost classes and the shapes `CostKey` can name are the same
   enumeration.
4. Silent builder: for every valid topology `BuildFFmpegArgs` emits that
   topology's encoder and filters, or an error - never another pipeline.
5. One grant: the class the producer bills at spawn equals
   `plan.Route.Class()`.
6. Byte-stable: `testdata/ladder_golden.txt` and `testdata/route_golden.txt`
   (decision, route, class, key, producer, runtime options, admission, hash)
   do not change across the migration.

## Migration

Additive first, code moves last. Landed 2026-09-17, one commit each, the
goldens byte-identical throughout: the route golden corpus; this ADR;
`Topology` with `EncodeRoute.Topology()` and the calibration matrix rebuilt on
it; `SourceSnapshot`; `ResolvedRoute` on the plan with the old derivations
removed and `AdmissionGrant` beside it; `RouteExpressibility` in shadow (the
resolver asks the port, a disagreement is logged and counted as
`evey_hlsvod_route_not_expressible_total`, the builder still falls back);
the identity / runtime / admission classification of every fingerprint field
proven by mutation (the `EncodeSpec`/`RuntimeOptions` split as a guard rather
than a struct change - the struct is built in 30 test literals); `HWPolicy`
read through one accessor; `HWFailureReport` carrying the exact route to the
fallback record; `SessionCost` as the one pricing derivation of spawn and
probe; the resolver split by topic and the domain's reading index.

One deliberate behaviour change surfaced by the corpus and committed on its
own: an HDR passthrough spilled to the CPU (pool full, blocked title) is now
shaped as the tone-map it runs - the HDR floor - where it was shaped as a
plain SDR ladder while billed and fingerprinted as software HDR.

Also landed: the admission pin key derives from the resolved route (its
codec and its title block, read before the pool spill - the spill is the pin's
answer, never its input), and the hardware policy is read once per `Resolve`
(`PlaybackRequest.Policy`, threaded through the route derivation like the
source snapshot; only the legacy wrappers still read the toggles themselves).

**Expressibility out of shadow (2026-09-18).** The exit criterion was
coverage, not time: every `{backend, topology, needs}` cell the live resolver
can reach on the production host (six on the QSV + VAAPI box: the QSV
zero-copy, decoupled, passthrough and burn-in+overlay, the VAAPI GPU
tone-map and the VAAPI zero-copy an interlaced source takes) checked at both
sites - one recalibration plus a corpus of real titles resolved per topology -
with zero disagreements. The corpus also found the one real bug: an mkvgo
`field_order` of `interlaced` no decision path read (now `library.ScanType`).
Since then `pickBackend` asks the port per candidate (topology and needs:
the compositor for a burn-in, the upload chain and device for a decoupled
decode, the GPU tone-map filter) and a refused backend is simply not in the
field; the resolver's mirrors (`burnInStaysSoftware`, the upload-chain clause
of `backendServes`) are gone. The builder asks the same port at spawn
(`RouteRefusal`, before an argument is built) and REFUSES with
`ErrRouteNotExpressible` - no software encode under a GPU session, no bridge
in place of a GPU tone-map the plan named. A refusal is an invariant
violation counted on `evey_hlsvod_route_not_expressible_total`, which must
stay at zero; the per-cell coverage table was the exit device and goes with
the shadow.

Still to do, each gated on something outside the code, in this order:

1. **Demotion by exact route** - when the calibration runner can probe one
   topology on one backend (today it probes an encode of a codec):
   `DemoteRoute` re-proves the failed `HWFailureReport.Route` and, on a
   confirmed failure, removes THAT proof (`HWRoute*`) from the backend rather
   than the codec. The report already carries the route.
2. **Legacy wrappers** - `EffectiveVideoCodec`, `ResolveHW`, `HWRouteUsable`,
   `LadderClassFor`, `ProducerFor`, `Fingerprint` and `ShapeLadder` called
   on their own, and the adapter's aliases, go once their tests are rewritten
   against `Resolve`; invariant 1 then becomes a contract guard.
3. **Direct / mkvgo / copy vs transcode** - the zero-process paths (mkvgo
   on-demand `od_`/`cc_`, the plan master) are dispatched before this
   resolver runs and the copy rung is a `Packager`, not a route; a single
   picture of "which engine serves which request" is a separate clarification,
   deliberately kept out of this ADR.

Not added, on purpose: a `Pricer` interface (`chargesFor` is already the one
pricing function), a struct split of `DecisionFingerprint` (the classification
guard removes the double source of truth the split was for), a route frozen
on the pin (a behaviour change to decide on its own).

## Consequences

- A new route or codec is declared once (a topology) and reaches the cost
  model, the calibration, the builder check and the fingerprint by
  construction.
- The "agrees with" tests and the mirror comments disappear with the
  duplicates they guarded.
- `ErrRouteNotExpressible` in production is a planner bug, visible as a
  counter, not a silent CPU encode under a GPU bill.
