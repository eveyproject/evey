package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
//  1. NewCalibrationService — wires the repo + runner; capability starts
//     at SafeFallback() (720p software, no HW) so requests served BEFORE
//     EnsureCalibrated finishes still work.
//  2. EnsureCalibrated(ctx) — called once at boot. Reads the repo,
//     re-bench if (a) nothing persisted yet, (b) CPU signature changed
//     (container migrated), or (c) caller forced refresh via
//     Recalibrate. Persists the result. Updates the in-memory snapshot.
//  3. Current() — lock-free read of the latest capability, used by the
//     HLS-VOD handler on every request.
//  4. Recalibrate(ctx) — admin endpoint trigger. Bypasses the cache,
//     reruns the benchmark, persists, updates the snapshot.
//
// Concurrency: Current/EnsureCalibrated/Recalibrate are safe to call
// concurrently. A single benchInFlight lock guarantees only one
// benchmark runs at a time (admin can't pile up requests by spamming
// the recalibrate button).
type CalibrationService struct {
	repo   domaintranscoding.CapabilityRepo
	runner domaintranscoding.BenchmarkRunner
	logger *slog.Logger

	current       atomic.Pointer[domaintranscoding.EncodeCapability]
	benchInFlight sync.Mutex

	// demoteMu serialises runtime capability demotions (level-2 safety net) so
	// concurrent encode failures don't race the read-modify-write of `current`.
	demoteMu sync.Mutex
	// fallbacks is a small ring of recent HW→lower-tier demotions, surfaced to
	// the admin (QoE dashboard) so a degraded host is visible without log diving.
	fallbacksMu sync.Mutex
	fallbacks   []HWFallbackEvent
}

// HWFallbackEvent records one runtime HW-encode demotion for the admin alert.
type HWFallbackEvent struct {
	Class string    `json:"class"`
	Codec string    `json:"codec"`
	At    time.Time `json:"at"`
}

const maxFallbackEvents = 20

func NewCalibrationService(repo domaintranscoding.CapabilityRepo, runner domaintranscoding.BenchmarkRunner, logger *slog.Logger) *CalibrationService {
	if logger == nil {
		logger = slog.Default()
	}
	s := &CalibrationService{
		repo:   repo,
		runner: runner,
		logger: logger,
	}
	fallback := domaintranscoding.SafeFallback()
	s.current.Store(&fallback)
	return s
}

// Current returns the in-memory capability — never nil, always
// includes at least the safe fallback. Read on every HLS-VOD request
// so MUST be fast (atomic pointer load).
func (s *CalibrationService) Current() domaintranscoding.EncodeCapability {
	return *s.current.Load()
}

// EnsureCalibrated runs at boot. If a valid persisted capability is
// found (CPU signature matches the current host), we use it. Otherwise
// we run the benchmark and persist the result. Blocks until the
// benchmark completes — callers can wrap it in a goroutine if they
// want a non-blocking boot.
func (s *CalibrationService) EnsureCalibrated(ctx context.Context) error {
	persisted, found, err := s.repo.Get(ctx)
	if err != nil {
		// Repo failure is non-fatal at boot — the safe fallback covers
		// the period until the next manual recalibration. We just log
		// loudly so an admin notices the DB isn't reachable.
		s.logger.Error("calibration: repo read failed at boot, staying on safe fallback",
			slog.Any("err", err))
		return nil
	}

	if found && persisted.SoftwareMaxHeight > 0 {
		currentSig := sysinfo.CPUSignature()
		if currentSig != "" && persisted.CPUSignature == currentSig {
			// A matching CPU signature does NOT guarantee the persisted HW backend
			// is usable here (shared DB across a HW + a non-HW container, a moved
			// disk, a driver regression). Re-validate it with a real encode probe;
			// if it's dead, strip HW and keep the still-valid software ceiling
			// rather than selecting a backend that fails at playtime and loops the
			// player on a ~30s timeout.
			if persisted.HWBackend != "" && persisted.HWMaxHeight > 0 && !s.runner.HWBackendUsable(ctx, persisted.HWBackend) {
				s.logger.Warn("calibration: persisted HW backend not usable on this host, falling back to software",
					slog.String("hw_backend", persisted.HWBackend))
				persisted.HWBackend = ""
				persisted.HWDevice = ""
				persisted.HWMaxHeight = 0
				persisted.HWFeatures = nil
			}
			s.logger.Info("calibration: using persisted capability",
				slog.Int("software_max_height", persisted.SoftwareMaxHeight),
				slog.Int("hw_max_height", persisted.HWMaxHeight),
				slog.String("hw_backend", persisted.HWBackend),
				slog.Time("benchmarked_at", persisted.BenchmarkedAt))
			s.current.Store(&persisted)
			return nil
		}
		s.logger.Info("calibration: persisted capability stale, re-benching",
			slog.String("persisted_sig", persisted.CPUSignature),
			slog.String("current_sig", currentSig))
	} else {
		s.logger.Info("calibration: no persisted capability, running first-boot benchmark")
	}

	return s.benchAndPersist(ctx)
}

// Recalibrate forces a fresh benchmark + persist, ignoring any cached
// capability. Used by the admin "recalibrate" button.
//
// Returns ErrBenchmarkBusy if another caller is already running the
// benchmark — the UI surfaces that as "already in progress" rather than
// queuing a second run.
func (s *CalibrationService) Recalibrate(ctx context.Context) error {
	return s.benchAndPersist(ctx)
}

// ErrBenchmarkBusy is returned when a second concurrent recalibration
// is attempted while the first hasn't finished yet.
var ErrBenchmarkBusy = errors.New("calibration: benchmark already in progress")

func (s *CalibrationService) benchAndPersist(ctx context.Context) error {
	if !s.benchInFlight.TryLock() {
		return ErrBenchmarkBusy
	}
	defer s.benchInFlight.Unlock()

	cap, err := s.runner.Run(ctx)
	if err != nil {
		return fmt.Errorf("calibration: benchmark failed: %w", err)
	}
	// Run() should always return at least the 720p floor — defensive
	// guard against a future runner change that returns 0.
	if cap.SoftwareMaxHeight <= 0 {
		cap.SoftwareMaxHeight = 720
	}
	if cap.BenchmarkedAt.IsZero() {
		cap.BenchmarkedAt = time.Now().UTC()
	}
	if cap.CPUSignature == "" {
		cap.CPUSignature = sysinfo.CPUSignature()
	}

	if err := s.repo.Set(ctx, cap); err != nil {
		s.logger.Warn("calibration: persist failed (in-memory still updated)",
			slog.Any("err", err))
		// We still publish to in-memory — request handling benefits
		// even if the DB write failed. Next boot will re-bench.
	}
	s.current.Store(&cap)
	return nil
}

// DemoteCapability is the level-2 safety net: after a hardware encode fails,
// the orchestrator calls this with the classified failure + the output codec.
// It applies the minimal-blast-radius PlanDemotion to the live capability —
// drop tuning on an unsupported option, remove just the codec on a missing
// encoder, disable HW only on a real init failure — persists deterministic
// demotions, and records the event for the admin alert. The next request's
// resolveHW then reads the demoted capability and resolves a lower tier, so the
// client's automatic 503-retry re-encodes at the working tier with no fork of
// the (encoder-agnostic) segment cache.
//
// Returns true when the capability actually changed. A no-op (already demoted)
// returns false, which naturally DEBOUNCES the storm of concurrent failures a
// single bad option produces: only the first call writes + alerts.
func (s *CalibrationService) DemoteCapability(ctx context.Context, class domaintranscoding.FailureClass, codec string) bool {
	if !class.IsHWCapability() {
		return false
	}
	s.demoteMu.Lock()
	defer s.demoteMu.Unlock()

	cur := *s.current.Load()
	d := domaintranscoding.PlanDemotion(cur, class, codec)
	if !d.Changed {
		return false // already demoted — debounces concurrent failures
	}

	s.recordFallback(class, codec)
	if d.Persist {
		if err := s.repo.Set(ctx, d.Capability); err != nil {
			s.logger.Warn("calibration: demote persist failed (in-memory still updated)",
				slog.Any("err", err))
		}
	}
	s.current.Store(&d.Capability)
	s.logger.Warn("calibration: HW capability demoted after encode failure",
		slog.String("class", string(class)),
		slog.String("codec", codec),
		slog.Bool("persist", d.Persist),
		slog.String("new_backend", d.Capability.HWBackend),
		slog.Int("new_hw_max_height", d.Capability.HWMaxHeight))
	return true
}

func (s *CalibrationService) recordFallback(class domaintranscoding.FailureClass, codec string) {
	s.fallbacksMu.Lock()
	defer s.fallbacksMu.Unlock()
	s.fallbacks = append(s.fallbacks, HWFallbackEvent{
		Class: string(class),
		Codec: codec,
		At:    time.Now().UTC(),
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
