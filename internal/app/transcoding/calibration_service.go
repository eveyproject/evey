package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// CalibrationService owns the boot-time benchmark + persistence and
// exposes the calibrated capability to the rest of the app.
//
// Lifecycle:
//
//  1. NewCalibrationService - wires the repo + runner; capability starts
//     at SafeFallback() (720p software, no HW) so requests served BEFORE
//     EnsureCalibrated finishes still work.
//  2. EnsureCalibrated(ctx) - called once at boot, NEVER benchmarks. Reads
//     the repo and publishes the best capability it can serve right now: the
//     persisted row when it is valid, the persisted row stripped of its HW
//     when the mode disagrees, the safe fallback when nothing usable exists.
//     When a bench is needed - nothing persisted, CPU signature changed
//     (container migrated), probes stale, mode mismatch - it is flagged
//     PENDING, not run: a boot benchmark blocked the whole server for
//     2-5 min (healthz 502) and measured the migrations, scans and backfills
//     that boot alongside it, not the box.
//  3. BenchWhenOpportune(ctx) - called from a tick loop. Runs the pending
//     (or provisional) bench once the host has been idle for a whole probe
//     window: idleProbes consecutive samples, idleProbeEvery apart, each
//     with no active stream and host CPU under idleCPUPercent. Dumb and
//     robust: a one-second dip between two scan batches cannot fake it, and
//     a loaded boot never yields 20 quiet seconds in a row.
//  4. Current() - lock-free read of the latest capability, used by the
//     HLS-VOD handler on every request.
//  5. Recalibrate(ctx) - admin endpoint trigger. Bypasses the cache and the
//     idle window, reruns the benchmark, persists, updates the snapshot.
//
// Concurrency: Current/EnsureCalibrated/Recalibrate are safe to call
// concurrently. A single benchInFlight lock guarantees only one
// benchmark runs at a time (admin can't pile up requests by spamming
// the recalibrate button).
// hwInitStrikeWindow: a second missed HW init inside this window demotes the GPU.
const hwInitStrikeWindow = 10 * time.Minute

// hwProofReuseWindow: a host proof that just PASSED answers the next failures
// of the same (backend, codec) without re-encoding. Concurrent sessions of one
// bad title all fail within seconds, and each would otherwise wait its turn on
// demoteMu to run the same multi-second probe and reach the same verdict.
const hwProofReuseWindow = 30 * time.Second

type CalibrationService struct {
	repo   domaintranscoding.CapabilityRepo
	runner domaintranscoding.BenchmarkRunner
	logger *slog.Logger
	// hwDevice is the operator's device pin (WithHWDevice), "" = none.
	hwDevice string
	// hwInitStrikeAt is the first missed HW init of the current window (under demoteMu).
	hwInitStrikeAt time.Time
	// hwProofOK remembers when a host proof last passed, per proof key (under
	// demoteMu); cleared by any demotion.
	hwProofOK map[string]time.Time

	// mode is the admin-selectable HW-acceleration policy (auto/off/nvenc/
	// vaapi/qsv). It projects to a HWPreference the runner honours: a backend
	// pin and/or a software-only flag. Guarded by modeMu because SetHWMode can
	// run concurrently with a boot EnsureCalibrated. Empty zero-value == auto.
	modeMu sync.Mutex
	mode   domaintranscoding.HWMode

	// swMaxOverride / swHDRMaxOverride force the software encode ceilings,
	// bypassing what the benchmark measured. 0 = use the calibrated value.
	// Wired from EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT / _SOFTWARE_HDR_MAX_HEIGHT
	// so an operator can empirically push a higher (HDR) rung on the real host
	// when the calibrated cap - which applies a 1.2x realtime margin and may be
	// decode-bound rather than output-resolution-bound - is more conservative
	// than the host actually sustains. Read-only knob; the runtime throttle and
	// the input pacing still protect a host that overshoots.
	swMaxOverride    int
	swHDRMaxOverride int

	// swAV1MaxOverride forces the AV1 software ceiling (0 = use the calibrated
	// value). Unlike the two ceilings above this is admin-settable at RUNTIME from
	// the server-config panel, so it is atomic: the settings handler writes it via
	// SetSoftwareAV1MaxOverride while Current() reads it lock-free on every request.
	swAV1MaxOverride atomic.Int64

	current       atomic.Pointer[domaintranscoding.EncodeCapability]
	benchInFlight sync.Mutex

	// benchmarking is true for the duration of a benchmark run. Background work
	// (intro detection) reads it via IsBenchmarking to SIGSTOP itself and leave the
	// CPU/memory bandwidth to the calibration - a bench contended by a 4K decode
	// would under-measure the host's true ceiling.
	benchmarking atomic.Bool

	// activeStreams reports the number of live transcode/playback sessions. The
	// idle-gate on Recalibrate refuses a MANUAL recalibration while streams are
	// active: a benchmark run alongside real transcodes measures the contended
	// throughput, not the host ceiling, and would pin the cap a rung too low (the
	// per-rung MEDIAN in the runner rejects a transient blip; this gate handles
	// SUSTAINED load the median can't). nil (unwired / tests) disables the gate.
	// First-boot calibration (EnsureCalibrated) is never gated - there are no
	// streams at boot and the host needs a cap. Wired once at startup, read later,
	// so no lock is needed (the wire→serve barrier establishes happens-before).
	activeStreams func() int

	// hostBusy samples the host-wide CPU busy share (0-100) right before a
	// benchmark. Everything on the box counts, which is the point: a bench that
	// starts on a host already pinned by an import or a neighbour is measuring
	// the neighbour. See WithHostLoadProbe; nil never reports contention.
	hostBusy func() float64

	// contendedMu guards contendedNextAt, the earliest moment the idle test
	// resumes after an attempt that failed or came back contended (a fixed
	// cooldown, see benchRetryCooldown) - never a growing backoff: the idle
	// window itself is the pacing, and the test keeps running until a trusted
	// calibration lands.
	contendedMu     sync.Mutex
	contendedNextAt time.Time

	// pendingBench is set by EnsureCalibrated when the persisted state calls
	// for a benchmark that boot deliberately did not run; pendingReason says
	// why (admin status). Cleared by the next successful bench, whoever ran it.
	pendingBench  atomic.Bool
	pendingMu     sync.Mutex
	pendingReason string
	// idleProbes / idleProbeEvery shape the opportune-moment window (see
	// BenchWhenOpportune); sleep is the seam tests replace.
	idleProbes     int
	idleProbeEvery time.Duration
	sleep          func(ctx context.Context, d time.Duration) error

	// currentMu serialises the read-modify-write of `current` between a runtime
	// demotion (load→PlanDemotion→store) and a (re)bench store. They take
	// different outer locks (demoteMu vs benchInFlight), so without this a demote
	// computed from a pre-bench snapshot could store back over a fresh bench
	// result (lost update). Always acquired LAST (inside demoteMu / benchInFlight,
	// which are disjoint) so no lock cycle. The hot-path Current() read stays
	// lock-free on the atomic pointer - this only guards the two rare writers.
	currentMu sync.Mutex

	// demoteMu serialises runtime capability demotions (level-2 safety net) so
	// concurrent encode failures don't race the read-modify-write of `current`.
	demoteMu sync.Mutex
	// fallbacks is a small ring of recent HW→lower-tier demotions, surfaced to
	// the admin (QoE dashboard) so a degraded host is visible without log diving.
	fallbacksMu sync.Mutex
	fallbacks   []HWFallbackEvent

	// changeCallbacks fire after every capability mutation that outlives boot
	// (persisted load, bench completion, runtime demotion) so dependents priced
	// on bench data - the video admission cost model above all - re-arm instead
	// of running on a dead snapshot. Callbacks run synchronously on the mutating
	// goroutine and MUST NOT call back into Recalibrate/DemoteCapability.
	changeMu        sync.Mutex
	changeCallbacks []func()
}

// HWFallbackEvent records one runtime HW session failure for the admin alert,
// with what became of it: the host was demoted, the host was re-confirmed and
// only the title fell back, or an init miss counted as a strike.
type HWFallbackEvent struct {
	Class   string    `json:"class"`
	Codec   string    `json:"codec"`
	At      time.Time `json:"at"`
	Verdict string    `json:"verdict,omitempty"`
	// Backend / Route name the exact route that failed (ADR-0030, 7); empty
	// on a report that did not carry one.
	Backend string `json:"backend,omitempty"`
	Route   string `json:"route,omitempty"`
}

const (
	fallbackVerdictHost       = "host_demoted"
	fallbackVerdictMediaRoute = "media_route"
	fallbackVerdictStrike     = "strike"
)

// OnCapabilityChange registers fn to run after every capability change that
// matters at runtime: persisted-capability load, benchmark completion and HW
// demotion. Registration normally happens once during wire, but is safe any
// time. fn must be fast and must not re-enter the calibration service's
// mutating paths.
func (s *CalibrationService) OnCapabilityChange(fn func()) {
	if fn == nil {
		return
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.changeCallbacks = append(s.changeCallbacks, fn)
}

func (s *CalibrationService) notifyCapabilityChange() {
	s.changeMu.Lock()
	cbs := slices.Clone(s.changeCallbacks)
	s.changeMu.Unlock()
	for _, fn := range cbs {
		fn()
	}
}

const maxFallbackEvents = 20

// fallbackDedupWindow collapses identical back-to-back failure events.
const fallbackDedupWindow = 10 * time.Second

// CalibrationOption configures a CalibrationService at construction.
type CalibrationOption func(*CalibrationService)

// WithHWMode sets the initial HW-acceleration mode (the bootstrap value derived
// from env + any persisted admin override). Empty/unknown is treated as auto.
// WithHWDevice pins the calibration to one device (render node / CUDA index),
// see HWPreference.Device. Empty = no pin.
func WithHWDevice(device string) CalibrationOption {
	return func(s *CalibrationService) { s.hwDevice = strings.TrimSpace(device) }
}

func WithHWMode(mode domaintranscoding.HWMode) CalibrationOption {
	return func(s *CalibrationService) {
		s.mode = mode
	}
}

// WithSoftwareCapOverrides forces the software encode ceilings (0 = no
// override). Lets an operator test a higher software / HDR rung than the
// benchmark calibrated, e.g. when the cap is decode-bound (output resolution
// barely changes throughput) and the 1.2x margin pinned it too low.
func WithSoftwareCapOverrides(swMax, swHDRMax int) CalibrationOption {
	return func(s *CalibrationService) {
		s.swMaxOverride = swMax
		s.swHDRMaxOverride = swHDRMax
	}
}

// WithSoftwareAV1MaxOverride sets the boot value of the admin-settable AV1
// software ceiling override (0 = none; AV1 stays hardware-only / calibrated).
// Loaded from the persisted server-config key at startup; changed live via
// SetSoftwareAV1MaxOverride from the admin panel.
func WithSoftwareAV1MaxOverride(h int) CalibrationOption {
	return func(s *CalibrationService) {
		if h > 0 {
			s.swAV1MaxOverride.Store(int64(h))
		}
	}
}

// WithHostLoadProbe installs the host-wide CPU busy sampler consulted before
// every benchmark and before every automatic re-bench. Production wires
// sysinfo.CPUBusyPercent over a one-second window; tests inject a constant.
func WithHostLoadProbe(probe func() float64) CalibrationOption {
	return func(s *CalibrationService) {
		s.hostBusy = probe
	}
}

// routeEvidenceFloorProbeVersion is the CapabilityProbeVersion from which route
// evidence covers every shape the software chain serves: measured at a route's
// ceiling rung (720p / 480p floors included, v11) and x265 measured on the
// tone-map route too (v12 - a HEVC-preferring client with no GPU lands on
// sw-hdr/libx265, which a v11 row never priced and admission refused). A
// software-only row older than this is re-benched once; later probe bumps
// are hardware-only and leave software-only rows alone.
const routeEvidenceFloorProbeVersion = 12

// idleProbes consecutive host samples, idleProbeEvery apart, must all read
// idle (no active stream, CPU under idleCPUPercent) before a pending or
// provisional benchmark runs: ~20 s of continuous calm, the "opportune moment".
const (
	idleProbes     = 5
	idleProbeEvery = 4 * time.Second
)

// contendedCPUPercent is the host-wide CPU busy share above which a benchmark
// is not measuring this host but whatever else runs on it. 50 % leaves the
// normal boot noise alone (the embedded Postgres and Meilisearch settling, a
// scanner ticking) and catches the case seen live: a library import plus its
// indexing pinning every core, so a host that idles at 1440p@59fps with a 4K
// GPU benched 720p@5fps, lost its GPU and armed a capacity of one stream.
const contendedCPUPercent = 50.0

// idleCPUPercent is the host-wide busy share under which a re-bench of a
// provisional calibration is worth its ~3 minutes of full CPU.
const idleCPUPercent = 20.0

// benchRetryCooldown is how long the idle test pauses after a bench that
// failed or came back contended (the host looked idle for the whole window,
// then got busy during the encodes). A fixed, short pause - not a doubling
// backoff that used to push the next test out by hours: as long as the host
// has no trusted calibration, the idle test must keep running, and a host
// that is really busy fails the window within 4 s at no cost.
const benchRetryCooldown = 5 * time.Minute

// SetSoftwareAV1MaxOverride updates the AV1 software ceiling override live (0
// clears it, restoring the calibrated value). Applied by Current() on the next
// request - no re-bench needed, the override just overlays the persisted cap.
func (s *CalibrationService) SetSoftwareAV1MaxOverride(h int) {
	if h < 0 {
		h = 0
	}
	s.swAV1MaxOverride.Store(int64(h))
}

// SoftwareAV1MaxOverride returns the current AV1 software ceiling override (0 =
// none). Read by the settings handler to render the panel value.
func (s *CalibrationService) SoftwareAV1MaxOverride() int {
	return int(s.swAV1MaxOverride.Load())
}

func NewCalibrationService(repo domaintranscoding.CapabilityRepo, runner domaintranscoding.BenchmarkRunner, logger *slog.Logger, opts ...CalibrationOption) *CalibrationService {
	if logger == nil {
		logger = slog.Default()
	}
	s := &CalibrationService{
		repo:   repo,
		runner: runner,
		logger: logger,
		mode:   domaintranscoding.HWModeAuto,
		// Real host load by default: a service built without the option must
		// still tell a bench run on a busy host from a trusted one.
		hostBusy:       func() float64 { return sysinfo.CPUBusyPercent(time.Second) },
		idleProbes:     idleProbes,
		idleProbeEvery: idleProbeEvery,
		sleep:          sleepCtx,
	}
	for _, opt := range opts {
		opt(s)
	}
	fallback := domaintranscoding.SafeFallback()
	s.current.Store(&fallback)
	return s
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// HWMode returns the active HW-acceleration mode (for the admin GET).
func (s *CalibrationService) HWMode() domaintranscoding.HWMode {
	s.modeMu.Lock()
	defer s.modeMu.Unlock()
	if s.mode == "" {
		return domaintranscoding.HWModeAuto
	}
	return s.mode
}

// preference snapshots the current mode as the runner-facing HWPreference,
// with the operator's device pin (WithHWDevice) layered on.
func (s *CalibrationService) preference() domaintranscoding.HWPreference {
	pref := s.HWMode().Preference()
	pref.Device = s.hwDevice
	return pref
}

// derivePolicy is the DerivePolicy that turns raw bench evidence into delivery
// ceilings. The production defaults today; kept as a seam so a future admin-tunable
// margin/floor is a one-line change here rather than threading config everywhere.
// Operator ceiling OVERRIDES stay out of it - they are a Current()-time overlay.
func (s *CalibrationService) derivePolicy() domaintranscoding.DerivePolicy {
	return domaintranscoding.DefaultDerivePolicy()
}

// SetHWMode applies a new admin-selected HW mode and re-benches so the change
// takes effect immediately (no restart). The caller is responsible for
// persisting the mode string (server_settings); this only drives the in-process
// calibration. The mode is recorded synchronously (so HWMode() reflects it at
// once) and the re-bench runs under benchInFlight - a concurrent recalibration
// yields ErrBenchmarkBusy, but the mode stays set so the persisted intent still
// wins on the next bench/restart.
func (s *CalibrationService) SetHWMode(ctx context.Context, mode domaintranscoding.HWMode) error {
	if mode == "" {
		mode = domaintranscoding.HWModeAuto
	}
	s.modeMu.Lock()
	prev := s.mode
	s.mode = mode
	s.modeMu.Unlock()

	s.logger.Info("calibration: HW mode changed, re-benching",
		slog.String("from", string(prev)), slog.String("to", string(mode)))

	return s.Recalibrate(ctx)
}

// Current returns the in-memory capability - never nil, always
// includes at least the safe fallback. Read on every HLS-VOD request
// so MUST be fast (atomic pointer load).
func (s *CalibrationService) Current() domaintranscoding.EncodeCapability {
	c := *s.current.Load()
	// Operator overrides (env): force the software ceilings regardless of the
	// benchmark. 0 = no override. SoftwareHDRMaxHeight only ever LOWERS the HDR
	// delivery height below SoftwareMaxHeight (see SoftwareProducer.Profiles), so
	// to push HDR above the SDR cap raise BOTH knobs.
	if s.swMaxOverride > 0 {
		c.SoftwareMaxHeight = s.swMaxOverride
	}
	if s.swHDRMaxOverride > 0 {
		c.SoftwareHDRMaxHeight = s.swHDRMaxOverride
	}
	// Admin-settable AV1 software ceiling (server-config panel). Lock-free read on
	// the hot path; >0 un-gates AV1 software at that height even when the bench
	// calibrated it as non-viable (0).
	if av1 := s.swAV1MaxOverride.Load(); av1 > 0 {
		c.SoftwareAV1MaxHeight = int(av1)
	}
	return c
}

// EnsureCalibrated runs at boot. If a valid persisted capability is
// found (CPU signature matches the current host), we use it. Otherwise
// we run the benchmark and persist the result. Blocks until the
// benchmark completes - callers can wrap it in a goroutine if they
// want a non-blocking boot.
func (s *CalibrationService) EnsureCalibrated(ctx context.Context) error {
	persisted, found, err := s.repo.Get(ctx)
	if err != nil {
		// Repo failure is non-fatal at boot - the safe fallback covers
		// the period until the next manual recalibration. We just log
		// loudly so an admin notices the DB isn't reachable.
		s.logger.Error("calibration: repo read failed at boot, staying on safe fallback",
			slog.Any("err", err))
		return nil
	}

	if found && persisted.SoftwareMaxHeight > 0 {
		currentSig := sysinfo.CPUSignature()
		// The persisted capability must be consistent with the ACTIVE HW mode,
		// otherwise it was written under a different policy (a sibling container
		// on another GPU path, or this host before the admin changed the mode).
		// When it disagrees, fall through to a fresh bench under the current mode.
		pref := s.preference()
		var modeMismatch bool
		switch {
		case pref.DisableHW:
			// mode=off but the cache still carries a HW backend.
			modeMismatch = persisted.HWBackend != ""
		case pref.Backend != "":
			// forced backend but the cache holds a different one.
			modeMismatch = persisted.HWBackend != pref.Backend
		default:
			// auto: re-bench only if HW exists on the box but the cache picked
			// none (e.g. it was written while the mode was off).
			modeMismatch = persisted.HWBackend == "" && len(persisted.DetectedBackends) > 0
		}
		if modeMismatch {
			s.logger.Info("calibration: persisted capability inconsistent with HW mode, re-benching",
				slog.String("mode", string(s.HWMode())),
				slog.String("persisted_backend", persisted.HWBackend),
				slog.Any("detected_backends", persisted.DetectedBackends))
		}
		// New HW probes since this row was benched: re-bench once so they populate.
		// A software-only row is re-benched only when its ROUTE EVIDENCE is the
		// stale part: before routeEvidenceFloorProbeVersion a route capped under
		// 1080p (the 480p HDR floor next to a 1080p SDR ceiling) had no sample,
		// and admission - which prices a named route only on evidence - refused
		// every software HDR play on an idle host. A row with no route evidence
		// at all prices on the table/counter and is not affected: kept as is.
		probesStale := persisted.ProbeVersion < domaintranscoding.CapabilityProbeVersion &&
			(persisted.HWBackend != "" || (len(persisted.RouteSamples) > 0 && persisted.ProbeVersion < routeEvidenceFloorProbeVersion))
		if probesStale {
			s.logger.Info("calibration: persisted capability predates the current HW probes, re-benching",
				slog.Int("probe_version", persisted.ProbeVersion),
				slog.Int("current", domaintranscoding.CapabilityProbeVersion))
		}
		if currentSig != "" && persisted.CPUSignature == currentSig && !modeMismatch && !probesStale {
			// A matching CPU signature does NOT guarantee the persisted HW backend
			// is usable here (shared DB across a HW + a non-HW container, a moved
			// disk, a driver regression). Re-validate it with a real encode probe;
			// if it's dead, strip HW and keep the still-valid software ceiling
			// rather than selecting a backend that fails at playtime and loops the
			// player on a ~30s timeout.
			// EVERY certified backend is re-validated, not just the primary: the
			// resolver picks a backend per route from Backends, so a dead secondary
			// left in the row would still be chosen (and fail) for the routes it
			// proved. A dead primary hands over to the next live backend.
			if persisted.HWBackend != "" && persisted.HWMaxHeight > 0 {
				for _, bc := range persisted.BackendCaps() {
					if s.runner.HWBackendUsable(ctx, bc.Backend, bc.Device) {
						continue
					}
					s.logger.Warn("calibration: persisted HW backend not usable on this host, dropping it",
						slog.String("hw_backend", bc.Backend), slog.String("device", bc.Device))
					persisted = persisted.WithoutBackend(bc.Backend)
				}
			}
			// Re-derive the ceilings from the persisted PROOF under the current
			// policy: a policy change (PolicyVersion bump, a new margin/floor)
			// re-caps this host from its last bench without re-benching. In-memory
			// only, matching the HW-revalidation strip above - boot never re-persists.
			// Safe against a persisted HW demotion: disableHW clears HWBackend
			// (⇒ Derive yields hw ceiling 0), and codec/feature demotions pass
			// through Report() unchanged. ONLY when there is proof: a row persisted
			// before BenchSamples existed carries ceilings but no samples - its
			// ceilings ARE the only evidence, so keep them verbatim and let the next
			// bench populate samples (matching the BenchSamples "re-bench to
			// populate" contract) rather than flooring a working host to 720p.
			if len(persisted.BenchSamples) > 0 {
				persisted = domaintranscoding.Derive(persisted.Report(), s.derivePolicy())
			}
			s.logger.Info("calibration: using persisted capability",
				slog.Int("software_max_height", persisted.SoftwareMaxHeight),
				slog.Int("hw_max_height", persisted.HWMaxHeight),
				slog.String("hw_backend", persisted.HWBackend),
				slog.Int("policy_version", persisted.PolicyVersion),
				slog.Time("benchmarked_at", persisted.BenchmarkedAt))
			if persisted.BenchContended {
				s.logger.Warn("calibration: persisted capability was benchmarked on a busy host - provisional, re-benching once the host is idle",
					slog.Int("host_busy_percent", persisted.BenchHostBusyPercent))
			}
			s.current.Store(&persisted)
			s.notifyCapabilityChange()
			return nil
		}
		// Not trusted as-is, but boot never benchmarks: serve the best of what
		// is there and flag the bench for the next opportune moment.
		switch {
		case currentSig != "" && persisted.CPUSignature != currentSig:
			// Another CPU's ceilings say nothing about this one: safe fallback.
			s.logger.Info("calibration: persisted capability is another host's, serving the safe fallback until the host is benchmarked",
				slog.String("persisted_sig", persisted.CPUSignature),
				slog.String("current_sig", currentSig))
			s.markPending("cpu signature changed")
		case modeMismatch && (pref.DisableHW || pref.Backend != ""):
			// The row's accelerator is not what the mode allows: keep its
			// software ceiling, drop its HW, bench for the pinned/off mode.
			for _, bc := range persisted.BackendCaps() {
				persisted = persisted.WithoutBackend(bc.Backend)
			}
			s.publishInterim(persisted, "hw mode changed")
		case modeMismatch:
			s.publishInterim(persisted, "accelerator detected but not calibrated")
		default:
			s.publishInterim(persisted, "calibration probes outdated")
		}
		return nil
	}
	s.logger.Info("calibration: no persisted capability, first-boot benchmark deferred to the first idle window")
	s.markPending("first boot")
	return nil
}

// publishInterim serves a persisted-but-stale capability (re-derived from its
// proof when it has one) while the bench that will replace it is pending.
func (s *CalibrationService) publishInterim(cap domaintranscoding.EncodeCapability, reason string) {
	if len(cap.BenchSamples) > 0 {
		cap = domaintranscoding.Derive(cap.Report(), s.derivePolicy())
	}
	s.logger.Info("calibration: serving the persisted capability until the host is re-benchmarked",
		slog.String("reason", reason),
		slog.Int("software_max_height", cap.SoftwareMaxHeight),
		slog.Int("hw_max_height", cap.HWMaxHeight),
		slog.String("hw_backend", cap.HWBackend))
	s.current.Store(&cap)
	s.notifyCapabilityChange()
	s.markPending(reason)
}

func (s *CalibrationService) markPending(reason string) {
	s.pendingMu.Lock()
	s.pendingReason = reason
	s.pendingMu.Unlock()
	s.pendingBench.Store(true)
}

// CalibrationStatus is the admin-facing lifecycle of the calibration.
type CalibrationStatus struct {
	// State: "benchmarking" (a bench runs now), "pending" (a bench is due and
	// waits for an idle window), "provisional" (benched on a busy host, will
	// re-bench when idle), "calibrated".
	State string `json:"state"`
	// Reason says why a bench is pending ("first boot", "calibration probes
	// outdated", ...); empty otherwise.
	Reason string `json:"reason,omitempty"`
}

// Status reports where the calibration stands, for the admin card.
func (s *CalibrationService) Status() CalibrationStatus {
	switch {
	case s.benchmarking.Load():
		return CalibrationStatus{State: "benchmarking"}
	case s.pendingBench.Load():
		s.pendingMu.Lock()
		defer s.pendingMu.Unlock()
		return CalibrationStatus{State: "pending", Reason: s.pendingReason}
	case s.Current().BenchContended:
		return CalibrationStatus{State: "provisional"}
	}
	return CalibrationStatus{State: "calibrated"}
}

// NeedsBench reports whether a bench is due (pending or provisional): the tick
// loop probes the host closely while true, lazily otherwise.
func (s *CalibrationService) NeedsBench() bool {
	return s.pendingBench.Load() || s.Current().BenchContended
}

// Recalibrate forces a fresh benchmark + persist, ignoring any cached
// capability. Used by the admin "recalibrate" button.
//
// Idle-gated: refuses with ErrBenchmarkBusyTranscoding while transcodes are
// active, since a benchmark contended by real streams measures degraded
// throughput and would pin the cap a rung too low. Returns ErrBenchmarkBusy if
// another benchmark is already running. The UI surfaces both as a retry hint.
func (s *CalibrationService) Recalibrate(ctx context.Context) error {
	if s.activeStreams != nil {
		if n := s.activeStreams(); n > 0 {
			s.logger.Info("calibration: recalibrate refused, transcodes active",
				slog.Int("active_streams", n))
			return fmt.Errorf("%w (%d active)", ErrBenchmarkBusyTranscoding, n)
		}
	}
	return s.benchAndPersist(ctx)
}

// SetActiveStreamChecker wires the live-session counter the Recalibrate idle-gate
// consults. Injected post-construction because the producer/selector that counts
// sessions is built after the calibration service. nil-safe; pass nil to disable.
// Call once at startup, before HTTP serving begins.
func (s *CalibrationService) SetActiveStreamChecker(fn func() int) {
	s.activeStreams = fn
}

// IsBenchmarking reports whether a calibration benchmark is running right now.
// Background CPU work (intro detection) consults it to suspend itself and leave
// the host to the bench.
func (s *CalibrationService) IsBenchmarking() bool {
	return s.benchmarking.Load()
}

// ErrBenchmarkBusy is returned when a second concurrent recalibration
// is attempted while the first hasn't finished yet.
var ErrBenchmarkBusy = errors.New("calibration: benchmark already in progress")

// ErrBenchmarkBusyTranscoding is returned when a manual recalibration is
// requested while transcodes are active. Calibrating under load measures
// contended throughput and would pin the cap too low, so we refuse and ask the
// operator to retry once playback/transcoding has stopped.
var ErrBenchmarkBusyTranscoding = errors.New("calibration: refusing to recalibrate while transcodes are active")

func (s *CalibrationService) benchAndPersist(ctx context.Context) error {
	if !s.benchInFlight.TryLock() {
		return ErrBenchmarkBusy
	}
	defer s.benchInFlight.Unlock()
	s.benchmarking.Store(true)
	defer s.benchmarking.Store(false)

	// Sampled BEFORE the first bench encode: the bench itself pins the CPU, so
	// only the load already there tells whether the host is free to be measured.
	hostBusy := s.hostBusyPercent()
	cap, err := s.runner.Run(ctx, s.preference())
	if err != nil {
		return fmt.Errorf("calibration: benchmark failed: %w", err)
	}
	if cap.BenchmarkedAt.IsZero() {
		cap.BenchmarkedAt = time.Now().UTC()
	}
	if cap.CPUSignature == "" {
		cap.CPUSignature = sysinfo.CPUSignature()
	}
	// The runner MEASURES (proof: BenchSamples, SourceRate, HW facts); the policy
	// that turns fps into delivery ceilings lives in the domain's Derive, in one
	// pure place. Deriving here (rather than trusting the ceilings the runner also
	// computes) makes the ceilings a projection of the proof + PolicyVersion, so a
	// policy change re-caps every host on the next boot without re-benching. Only
	// when there IS proof: a runner that returns bare ceilings without samples (a
	// legacy path, a test double) has no evidence to derive from - trust its
	// ceilings as the only signal rather than flooring everything.
	if len(cap.BenchSamples) > 0 {
		cap = domaintranscoding.Derive(cap.Report(), s.derivePolicy())
	}
	// Derive already floors software to the policy's SoftwareFloor (720); keep a
	// belt against a future policy with a zero floor wedging every viewer.
	if cap.SoftwareMaxHeight <= 0 {
		cap.SoftwareMaxHeight = 720
	}
	// A bench started on a busy host measured the neighbours, not the box: keep
	// the result (it is the best evidence there is right now) but mark it
	// provisional so RebenchIfContended replaces it once the host is idle and
	// the admin view does not present a shared-box floor as this box's ceiling.
	cap.BenchHostBusyPercent = int(math.Round(hostBusy))
	cap.BenchContended = hostBusy >= contendedCPUPercent
	if cap.BenchContended {
		s.logger.Warn("calibration: host was already busy when the benchmark started - result is provisional, will re-bench once the host is idle",
			slog.Int("host_busy_percent", cap.BenchHostBusyPercent),
			slog.Float64("threshold_percent", contendedCPUPercent),
			slog.Int("software_max_height", cap.SoftwareMaxHeight),
			slog.Int("hw_max_height", cap.HWMaxHeight))
	}

	// Hold currentMu across BOTH the DB persist and the in-memory store so this
	// bench can't interleave with a concurrent DemoteCapability (which holds
	// currentMu over its own load→Set→store): otherwise the two repo.Set calls
	// race and the DB row can diverge from `current` - e.g. a boot-time HW
	// demotion gets clobbered by a bench landing its repo.Set afterwards.
	// DemoteCapability takes demoteMu before currentMu; this path takes only
	// currentMu, so there is no inverse lock ordering.
	s.currentMu.Lock()
	defer s.currentMu.Unlock()
	if err := s.repo.Set(ctx, cap); err != nil {
		s.logger.Warn("calibration: persist failed (in-memory still updated)",
			slog.Any("err", err))
		// We still publish to in-memory - request handling benefits
		// even if the DB write failed. Next boot will re-bench.
	}
	s.current.Store(&cap)
	s.pendingBench.Store(false)
	s.notifyCapabilityChange()
	return nil
}

// hostBusyPercent answers the host-wide CPU busy share, 0 when no probe is wired.
func (s *CalibrationService) hostBusyPercent() float64 {
	if s.hostBusy == nil {
		return 0
	}
	return s.hostBusy()
}

// BenchWhenOpportune runs the bench the host is waiting for - the one boot
// deferred (pending) or the replacement of a provisional calibration (benched
// while the host was busy) - as soon as the host has been idle for a whole
// probe window (hostIdleWindow). Meant for a tick loop; a check that fails
// costs at most the window. Reports whether a trusted calibration landed.
//
// A bench that fails or comes back contended (the host got busy between the
// window and the encodes) pauses the test for benchRetryCooldown, then the
// idle test resumes - it keeps running until a trusted calibration lands.
func (s *CalibrationService) BenchWhenOpportune(ctx context.Context) bool {
	if s.benchmarking.Load() {
		return false
	}
	pending, contended := s.pendingBench.Load(), s.Current().BenchContended
	if !pending && !contended {
		return false
	}
	s.contendedMu.Lock()
	tooSoon := time.Now().Before(s.contendedNextAt)
	s.contendedMu.Unlock()
	if tooSoon {
		return false
	}
	if !s.hostIdleWindow(ctx) {
		return false
	}
	reason := "provisional calibration"
	if pending {
		s.pendingMu.Lock()
		reason = s.pendingReason
		s.pendingMu.Unlock()
	}
	s.logger.Info("calibration: host idle for the whole probe window, benchmarking",
		slog.String("reason", reason), slog.Int("probes", s.idleProbes), slog.Duration("probe_every", s.idleProbeEvery))
	if err := s.benchAndPersist(ctx); err != nil {
		s.logger.Warn("calibration: opportune benchmark failed", slog.Any("err", err))
		s.armRetryCooldown()
		return false
	}
	if s.Current().BenchContended {
		s.armRetryCooldown()
		return false
	}
	s.contendedMu.Lock()
	s.contendedNextAt = time.Time{}
	s.contendedMu.Unlock()
	return true
}

// RebenchIfContended is BenchWhenOpportune under its former name.
func (s *CalibrationService) RebenchIfContended(ctx context.Context) bool {
	return s.BenchWhenOpportune(ctx)
}

// hostIdleWindow reports whether the host stayed idle for idleProbes
// consecutive samples idleProbeEvery apart: no active stream and host-wide
// CPU under idleCPUPercent at every one of them. The first busy sample ends
// the window early.
func (s *CalibrationService) hostIdleWindow(ctx context.Context) bool {
	for i := range s.idleProbes {
		if ctx.Err() != nil {
			return false
		}
		if s.activeStreams != nil && s.activeStreams() > 0 {
			return false
		}
		if busy := s.hostBusyPercent(); busy >= idleCPUPercent {
			s.logger.Debug("calibration: host busy, the idle window restarts",
				slog.Int("probe", i+1), slog.Float64("host_busy_percent", busy), slog.Float64("idle_threshold_percent", idleCPUPercent))
			return false
		}
		if i+1 < s.idleProbes {
			if err := s.sleep(ctx, s.idleProbeEvery); err != nil {
				return false
			}
		}
	}
	return true
}

// armRetryCooldown pauses the idle test for benchRetryCooldown; it resumes
// on its own afterwards (the loop keeps ticking while NeedsBench is true).
func (s *CalibrationService) armRetryCooldown() {
	s.contendedMu.Lock()
	defer s.contendedMu.Unlock()
	s.contendedNextAt = time.Now().Add(benchRetryCooldown)
	s.logger.Info("calibration: benchmark did not land a trusted calibration, the idle test resumes after a cooldown",
		slog.Duration("retry_in", benchRetryCooldown))
}

// DemoteCapability is the level-2 safety net: after a hardware session fails,
// the orchestrator calls this with the classified failure + the output codec.
//
// A failed session is not a verdict on the host. Before anything host-wide
// changes, the failure has to be CONFIRMED by a proof independent of the media
// that failed: a synthetic encode through the same backend (HWEncodeUsable),
// with the same probed tuning features rendered as production renders them.
// A host that passes keeps its capability untouched - the failure was the
// media's or the route's (the orchestrator has already sent that title to the
// CPU) - and the event is recorded for the admin with that verdict. Only a
// host that fails its own probe is demoted, through the minimal-blast-radius
// PlanDemotion (features narrowed to the ones re-confirmed, one codec removed,
// HW disabled), persisted when deterministic.
//
// Returns true when the capability actually changed. A no-op (already
// demoted, or the host confirmed) returns false, which naturally DEBOUNCES the
// storm of concurrent failures a single title produces.
func (s *CalibrationService) DemoteCapability(ctx context.Context, class domaintranscoding.FailureClass, codec string) bool {
	return s.DemoteRoute(ctx, HWFailureReport{Class: class, Codec: codec})
}

// DemoteRoute is DemoteCapability with the exact route that failed: the
// verdict rules are unchanged (keyed on class and codec), the route reaches
// the fallback record the admin panel shows.
func (s *CalibrationService) DemoteRoute(ctx context.Context, report HWFailureReport) bool {
	class, codec := report.Class, report.Codec
	if class == domaintranscoding.FailureRouteIncompatible {
		// The media and the route disagree; nothing to prove about the host.
		s.recordFallback(report, fallbackVerdictMediaRoute)
		return false
	}
	if !class.IsHWCapability() {
		return false
	}
	s.demoteMu.Lock()
	defer s.demoteMu.Unlock()

	cur := *s.current.Load()
	if cur.HWBackend == "" {
		return false // already software - nothing left to demote
	}

	switch class {
	case domaintranscoding.FailureHWUnsupportedOption:
		// Narrow the tuning to what the host re-proves feature by feature,
		// instead of wiping it all on the word of one session.
		proof := hwProofKey(cur.HWBackend, codec, "features")
		if s.hostProvenRecentlyLocked(proof) {
			s.recordFallback(report, fallbackVerdictMediaRoute)
			return false
		}
		kept := s.runner.ConfirmHWFeatures(ctx, cur.HWBackend, cur.HWDevice, codec, cur.HWFeatures)
		d := domaintranscoding.PlanFeatureDemotion(cur, kept)
		if !d.Changed {
			s.markHostProvenLocked(proof)
			s.recordFallback(report, fallbackVerdictMediaRoute)
			s.logger.Warn("calibration: HW tuning re-confirmed by a media-independent probe, the failure was the media's; host keeps its features",
				slog.String("codec", codec), slog.Any("features", cur.HWFeatures))
			return false
		}
		return s.applyDemotion(ctx, cur, d, report)

	case domaintranscoding.FailureHWDeviceMemory:
		// Transient by definition: the session fell back, the host is not touched.
		return false
	}

	// Init / encoder missing / corrupt output: the backend has to fail a real
	// encode of a synthetic clip before it is believed dead.
	proof := hwProofKey(cur.HWBackend, codec, "encode")
	if s.hostProvenRecentlyLocked(proof) {
		s.recordFallback(report, fallbackVerdictMediaRoute)
		return false
	}
	if s.runner.HWEncodeUsable(ctx, cur.HWBackend, cur.HWDevice, codec, nil) {
		s.markHostProvenLocked(proof)
		s.recordFallback(report, fallbackVerdictMediaRoute)
		s.logger.Warn("calibration: HW backend re-confirmed by a media-independent probe, the failure was the media's; host keeps its GPU",
			slog.String("class", string(class)), slog.String("codec", codec), slog.String("backend", cur.HWBackend))
		return false
	}

	// One failed probe is a slow start as often as a dead GPU (prod 2026-09-10:
	// a 4K seek during boot load). Two within the window prove the GPU.
	if class == domaintranscoding.FailureHWInit {
		now := time.Now()
		if now.Sub(s.hwInitStrikeAt) > hwInitStrikeWindow {
			s.hwInitStrikeAt = now
			s.recordFallback(report, fallbackVerdictStrike)
			s.logger.Warn("calibration: HW init missed once and the probe failed too, keeping the GPU until a second miss",
				slog.String("codec", codec), slog.Duration("window", hwInitStrikeWindow))
			return false
		}
		s.hwInitStrikeAt = time.Time{}
	}

	d := domaintranscoding.PlanDemotion(cur, class, codec)
	if !d.Changed {
		return false // already demoted - debounces concurrent failures
	}
	return s.applyDemotion(ctx, cur, d, report)
}

func hwProofKey(backend, codec, probe string) string { return backend + "/" + codec + "/" + probe }

// hostProvenRecentlyLocked reports whether the proof passed inside
// hwProofReuseWindow. Caller holds demoteMu.
func (s *CalibrationService) hostProvenRecentlyLocked(key string) bool {
	at, ok := s.hwProofOK[key]
	return ok && time.Since(at) < hwProofReuseWindow
}

func (s *CalibrationService) markHostProvenLocked(key string) {
	if s.hwProofOK == nil {
		s.hwProofOK = make(map[string]time.Time)
	}
	s.hwProofOK[key] = time.Now()
}

// applyDemotion stores a confirmed demotion (in memory + persisted when the
// plan says so), records it and notifies dependents. Called under demoteMu.
func (s *CalibrationService) applyDemotion(ctx context.Context, cur domaintranscoding.EncodeCapability, d domaintranscoding.Demotion, report HWFailureReport) bool {
	class, codec := report.Class, report.Codec
	// The capability is about to change: no earlier proof describes it.
	s.hwProofOK = nil
	// Hold currentMu across the whole load→store so a concurrent benchAndPersist
	// store can't land between the snapshot we demote from and the store back
	// (which would either clobber the fresh bench or be clobbered by it).
	s.currentMu.Lock()
	defer s.currentMu.Unlock()

	s.recordFallback(report, fallbackVerdictHost)
	// Stamp a demotion that actually switched the backend OFF, so it can expire
	// and be re-probed (ReprobeDemotedHW) instead of outliving its cause. A
	// demotion that only trimmed features or one codec leaves HW working and is
	// not on the clock.
	if cur.HWBackend != "" && d.Capability.HWBackend == "" {
		d.Capability.HWDemotedAt = time.Now().UTC()
	}
	if d.Persist {
		if err := s.repo.Set(ctx, d.Capability); err != nil {
			s.logger.Warn("calibration: demote persist failed (in-memory still updated)",
				slog.Any("err", err))
		}
	}
	s.current.Store(&d.Capability)
	s.notifyCapabilityChange()
	s.logger.Warn("calibration: HW capability demoted after a failure the host confirmed",
		slog.String("class", string(class)),
		slog.String("codec", codec),
		slog.Bool("persist", d.Persist),
		slog.String("new_backend", d.Capability.HWBackend),
		slog.Int("new_hw_max_height", d.Capability.HWMaxHeight),
		slog.Any("features", d.Capability.HWFeatures))
	return true
}

// HWDemotionTTL is how long a runtime HW demotion is trusted before the
// backend is re-probed. Short on purpose: the failures that trigger a demotion
// are overwhelmingly transient or source-specific (a busy GPU, a driver
// hiccup, a source this iGPU cannot decode), while the cost of believing one
// for too long is the whole host running on CPU. Measured cost of the
// unbounded version in production: ~27 hours of software-only playback for
// every user, from one 4K HEVC play.
const HWDemotionTTL = 30 * time.Minute

// ReprobeDemotedHW re-validates a hardware backend that a runtime failure
// switched off, once the demotion is older than HWDemotionTTL. Returns true
// when HW was restored.
//
// A demotion is a REACTION to one failure, not a measurement of the host, so it
// is never simply forgiven on a timer: the backend has to pass the same real
// encode probe boot uses (HWBackendUsable) before anything is restored, and the
// restore itself goes through a full re-bench so the ceilings come back from
// evidence rather than from the pre-demotion values. A backend that is really
// dead fails the probe and stays demoted, with the clock reset so the next
// attempt is another TTL away rather than every tick.
func (s *CalibrationService) ReprobeDemotedHW(ctx context.Context) bool {
	cur := *s.current.Load()
	if cur.HWDemotedAt.IsZero() || cur.HWBackend != "" {
		return false // never demoted, or already running on hardware again
	}
	if time.Since(cur.HWDemotedAt) < HWDemotionTTL {
		return false
	}
	// An operator who turned HW off must not have it probed back on.
	pref := s.preference()
	if pref.DisableHW {
		return false
	}
	candidates := cur.DetectedBackends
	if pref.Backend != "" {
		candidates = []string{pref.Backend}
	}
	for _, backend := range candidates {
		if !s.runner.HWBackendUsable(ctx, backend, pref.Device) {
			continue
		}
		s.logger.Info("calibration: demoted HW backend passed re-probe, re-benching",
			slog.String("backend", backend),
			slog.Time("demoted_at", cur.HWDemotedAt))
		if err := s.Recalibrate(ctx); err != nil {
			s.logger.Warn("calibration: re-bench after HW re-probe failed, staying software",
				slog.Any("err", err))
			return false
		}
		return true
	}
	// Still dead: reset the clock, one probe per TTL rather than one per tick.
	s.currentMu.Lock()
	latest := *s.current.Load()
	if !latest.HWDemotedAt.IsZero() && latest.HWBackend == "" {
		latest.HWDemotedAt = time.Now().UTC()
		s.current.Store(&latest)
	}
	s.currentMu.Unlock()
	s.logger.Info("calibration: demoted HW backend still not usable, staying software",
		slog.Any("candidates", candidates))
	return false
}

func (s *CalibrationService) recordFallback(report HWFailureReport, verdict string) {
	class, codec := report.Class, report.Codec
	s.fallbacksMu.Lock()
	defer s.fallbacksMu.Unlock()
	now := time.Now().UTC()
	// A storm of identical failures from one title (every rung, every retry)
	// is one event on the dashboard, not twenty.
	if n := len(s.fallbacks); n > 0 {
		last := s.fallbacks[n-1]
		if last.Class == string(class) && last.Codec == codec && last.Verdict == verdict && now.Sub(last.At) < fallbackDedupWindow {
			return
		}
	}
	s.fallbacks = append(s.fallbacks, HWFallbackEvent{
		Class:   string(class),
		Codec:   codec,
		At:      now,
		Verdict: verdict,
		Backend: report.Route.Backend,
		Route:   string(report.Route.Route),
	})
	if len(s.fallbacks) > maxFallbackEvents {
		s.fallbacks = s.fallbacks[len(s.fallbacks)-maxFallbackEvents:]
	}
}

// RecentFallbacks returns a copy of the recent HW-demotion events (newest last),
// for the admin QoE dashboard.
func (s *CalibrationService) RecentFallbacks() []HWFallbackEvent {
	s.fallbacksMu.Lock()
	defer s.fallbacksMu.Unlock()
	out := make([]HWFallbackEvent, len(s.fallbacks))
	copy(out, s.fallbacks)
	return out
}
