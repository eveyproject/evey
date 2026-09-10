package transcoding

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// fakeCapabilityRepo is a thread-safe in-memory CapabilityRepo for tests.
type fakeCapabilityRepo struct {
	mu     sync.Mutex
	stored *domaintranscoding.EncodeCapability
	getErr error
	setErr error
}

func (r *fakeCapabilityRepo) Get(_ context.Context) (domaintranscoding.EncodeCapability, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return domaintranscoding.EncodeCapability{}, false, r.getErr
	}
	if r.stored == nil {
		return domaintranscoding.EncodeCapability{}, false, nil
	}
	return *r.stored, true, nil
}

func (r *fakeCapabilityRepo) Set(_ context.Context, cap domaintranscoding.EncodeCapability) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setErr != nil {
		return r.setErr
	}
	r.stored = &cap
	return nil
}

// fakeBenchmarkRunner returns a configurable EncodeCapability and
// counts invocations so tests can prove the bench was / wasn't called.
type fakeBenchmarkRunner struct {
	calls  atomic.Int64
	result domaintranscoding.EncodeCapability
	delay  time.Duration
	err    error
	// hwUnusable inverts HWBackendUsable: default (false) reports HW usable so
	// existing persisted-HW tests keep their backend; set true to exercise the
	// re-validation strip path.
	hwUnusable bool
	// lastPref captures the preference the most recent Run was called with, so
	// tests can assert the mode projected to the expected backend pin / disable.
	lastPref atomic.Pointer[domaintranscoding.HWPreference]
}

func (r *fakeBenchmarkRunner) Run(ctx context.Context, pref domaintranscoding.HWPreference) (domaintranscoding.EncodeCapability, error) {
	r.calls.Add(1)
	r.lastPref.Store(&pref)
	if r.delay > 0 {
		select {
		case <-time.After(r.delay):
		case <-ctx.Done():
			return domaintranscoding.EncodeCapability{}, ctx.Err()
		}
	}
	return r.result, r.err
}

func (r *fakeBenchmarkRunner) HWBackendUsable(_ context.Context, _ string) bool {
	return !r.hwUnusable
}

func TestCalibration_Current_StartsAtSafeFallback(t *testing.T) {
	svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, &fakeBenchmarkRunner{}, nil)
	cap := svc.Current()
	assert.Equal(t, 720, cap.SoftwareMaxHeight, "must start at the safe fallback so requests pre-EnsureCalibrated still work")
}

func TestCalibration_EnsureCalibrated_FirstBootRunsBench(t *testing.T) {
	repo := &fakeCapabilityRepo{}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080,
			HWMaxHeight:       2160,
			HWBackend:         "nvenc",
			CPUSignature:      "fake-sig",
		},
	}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.EqualValues(t, 1, runner.calls.Load(), "benchmark must run on first boot")
	assert.Equal(t, 1080, svc.Current().SoftwareMaxHeight, "Current() must reflect the fresh bench result")
	// Persistence must happen so the next boot skips the bench.
	persisted, found, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 1080, persisted.SoftwareMaxHeight)
}

func TestCalibration_EnsureCalibrated_CachedMatch_SkipsBench(t *testing.T) {
	// Persist a capability whose CPU signature MATCHES the host's,
	// then check that EnsureCalibrated doesn't re-run the benchmark.
	// We grab the real host signature from sysinfo so the equality
	// check passes - the test relies on /proc/cpuinfo being stable.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1440,
			HWMaxHeight:       0,
			CPUSignature:      hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.Zero(t, runner.calls.Load(), "matching CPU sig must skip the benchmark")
	assert.Equal(t, 1440, svc.Current().SoftwareMaxHeight, "Current() must reflect the persisted cap")
}

func TestCalibration_EnsureCalibrated_ReDerivesFromPersistedProof(t *testing.T) {
	// A persisted capability carries PROOF (bench samples). Its stored ceiling is
	// stale (1440) but its evidence only sustains 1080. On boot we must re-derive
	// the ceiling from the proof under the current policy - no re-bench - and stamp
	// PolicyVersion. This is the payoff of the proof/policy split: a policy change
	// (or a stale row) re-caps every host from its last bench deterministically.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1440, // stale ceiling not backed by the evidence below
			CPUSignature:      hostSig,
			SourceRate:        24,
			BenchSamples: []domaintranscoding.BenchRungSample{
				{Ladder: domaintranscoding.LadderSoftware, Height: 720, FPS: 100, Passing: true},
				{Ladder: domaintranscoding.LadderSoftware, Height: 1080, FPS: 50, Passing: true},
				{Ladder: domaintranscoding.LadderSoftware, Height: 1440, FPS: 10, Passing: false},
			},
		},
	}
	runner := &fakeBenchmarkRunner{}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.Zero(t, runner.calls.Load(), "re-derivation must NOT re-bench")
	cur := svc.Current()
	assert.Equal(t, 1080, cur.SoftwareMaxHeight, "ceiling re-derived from the persisted proof, not the stale stored value")
	assert.Equal(t, domaintranscoding.PolicyVersion, cur.PolicyVersion, "the current policy version must be stamped on re-derive")
}

func TestCalibration_EnsureCalibrated_LegacyRowNoSamples_KeepsCeilings(t *testing.T) {
	// A row persisted before BenchSamples existed has ceilings but NO proof. Its
	// ceilings are the only evidence, so re-derivation must be skipped and the
	// stored values kept verbatim - never floored to 720p on a host that really
	// sustains more. PolicyVersion stays 0 (unstamped) since nothing was derived.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight:    1440,
			SoftwareHDRMaxHeight: 1080,
			CPUSignature:         hostSig,
			// no BenchSamples, no SourceRate, no PolicyVersion
		},
	}
	runner := &fakeBenchmarkRunner{}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.Zero(t, runner.calls.Load(), "matching sig must skip the bench")
	cur := svc.Current()
	assert.Equal(t, 1440, cur.SoftwareMaxHeight, "legacy ceiling kept verbatim (no proof to re-derive from)")
	assert.Equal(t, 1080, cur.SoftwareHDRMaxHeight)
	assert.Zero(t, cur.PolicyVersion, "nothing derived => PolicyVersion left unstamped")
}

func TestCalibration_SoftwareCapOverrides_ForceCurrentCeilings(t *testing.T) {
	// An operator override raises the software / HDR ceilings past what the
	// benchmark calibrated, so Current() must reflect the override regardless of
	// the persisted/benchmarked value (the empirical "try 720p HDR" knob).
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight:    720,
			SoftwareHDRMaxHeight: 480, // benchmark floored HDR to 480p
			CPUSignature:         hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{}

	// No override → calibrated values stand.
	base := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, base.EnsureCalibrated(context.Background()))
	assert.Equal(t, 480, base.Current().SoftwareHDRMaxHeight, "without override the calibrated HDR cap stands")

	// Override → forced ceilings.
	svc := newIdleHostCalibrationService(repo, runner, nil, WithSoftwareCapOverrides(1080, 720))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))
	cur := svc.Current()
	assert.Equal(t, 1080, cur.SoftwareMaxHeight, "override forces the software ceiling")
	assert.Equal(t, 720, cur.SoftwareHDRMaxHeight, "override forces the HDR ceiling (empirical 720p HDR test)")

	// Zero values = no override (each knob independent).
	hdrOnly := newIdleHostCalibrationService(repo, runner, nil, WithSoftwareCapOverrides(0, 720))
	require.NoError(t, hdrOnly.EnsureCalibrated(context.Background()))
	assert.Equal(t, 720, hdrOnly.Current().SoftwareMaxHeight, "0 software override leaves the calibrated value")
	assert.Equal(t, 720, hdrOnly.Current().SoftwareHDRMaxHeight, "HDR override applies independently")
}

func TestCalibration_SoftwareAV1MaxOverride_AdminSettableLive(t *testing.T) {
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight:    1080,
			SoftwareAV1MaxHeight: 0, // bench found AV1 software non-viable here
			CPUSignature:         hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{}

	// No override → the calibrated 0 stands (AV1 stays hardware-only via the gate).
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))
	assert.Equal(t, 0, svc.Current().SoftwareAV1MaxHeight)
	assert.Equal(t, 0, svc.SoftwareAV1MaxOverride())

	// Admin sets it live → applied on the next Current() read, no re-bench.
	svc.SetSoftwareAV1MaxOverride(720)
	assert.Equal(t, 720, svc.SoftwareAV1MaxOverride())
	assert.Equal(t, 720, svc.Current().SoftwareAV1MaxHeight, "live override must overlay the calibrated ceiling")

	// Clearing it (0) restores the calibrated value.
	svc.SetSoftwareAV1MaxOverride(0)
	assert.Equal(t, 0, svc.Current().SoftwareAV1MaxHeight)

	// Boot-time override option seeds the same field.
	booted := newIdleHostCalibrationService(repo, runner, nil, WithSoftwareAV1MaxOverride(1080))
	require.NoError(t, booted.EnsureCalibrated(context.Background()))
	assert.Equal(t, 1080, booted.Current().SoftwareAV1MaxHeight)
}

func TestCalibration_EnsureCalibrated_PersistedHWUnusable_StripsToSoftware(t *testing.T) {
	// Matching CPU sig but the persisted HW backend is no longer usable here
	// (e.g. shared DB across a HW and a non-HW container). EnsureCalibrated must
	// keep the software ceiling and strip the dead HW rather than selecting a
	// backend that fails at playtime and loops the player on a timeout.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080,
			HWMaxHeight:       2160,
			HWBackend:         "nvenc",
			HWDevice:          "/dev/dri/renderD128",
			HWFeatures:        []string{"rc-lookahead"},
			CPUSignature:      hostSig,
			ProbeVersion:      domaintranscoding.CapabilityProbeVersion,
		},
	}
	runner := &fakeBenchmarkRunner{hwUnusable: true} // HW dead on this host
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.Zero(t, runner.calls.Load(), "matching sig still skips the benchmark")
	cur := svc.Current()
	assert.Equal(t, 1080, cur.SoftwareMaxHeight, "software ceiling preserved")
	assert.Equal(t, 0, cur.HWMaxHeight, "dead HW stripped")
	assert.Empty(t, cur.HWBackend, "dead HW backend stripped")
	assert.Empty(t, cur.HWFeatures, "HW features stripped")
}

func TestCalibration_EnsureCalibrated_PersistedHWUsable_Kept(t *testing.T) {
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc", CPUSignature: hostSig, ProbeVersion: domaintranscoding.CapabilityProbeVersion,
		},
	}
	runner := &fakeBenchmarkRunner{} // hwUnusable=false → usable here
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	cur := svc.Current()
	assert.Equal(t, 2160, cur.HWMaxHeight, "usable HW kept")
	assert.Equal(t, "nvenc", cur.HWBackend, "usable HW backend kept")
}

func TestCalibration_EnsureCalibrated_ModePinMismatch_Rebench(t *testing.T) {
	// Matching CPU sig, but this instance's mode forces a DIFFERENT backend than
	// the persisted one (a sibling container on another GPU path wrote the cache
	// to the shared DB). The mode must win: re-bench instead of trusting the cache.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc", CPUSignature: hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 1080, HWBackend: "vaapi", CPUSignature: hostSig,
		},
	}
	svc := newIdleHostCalibrationService(repo, runner, nil, WithHWMode(domaintranscoding.HWModeVAAPI))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.EqualValues(t, 1, runner.calls.Load(), "mode pin mismatch must force a fresh bench, not trust the sibling's nvenc cache")
	assert.Equal(t, "vaapi", svc.Current().HWBackend, "Current() must reflect the pinned backend after re-bench")
	if p := runner.lastPref.Load(); assert.NotNil(t, p) {
		assert.Equal(t, "vaapi", p.Backend, "the bench must be pinned to vaapi")
		assert.False(t, p.DisableHW)
	}
}

func TestCalibration_EnsureCalibrated_ModePinMatch_SkipsBench(t *testing.T) {
	// Mode forces the SAME backend the cache holds → the cache is trusted as usual.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 1080, HWBackend: "vaapi", CPUSignature: hostSig, ProbeVersion: domaintranscoding.CapabilityProbeVersion,
		},
	}
	runner := &fakeBenchmarkRunner{} // hwUnusable=false → usable here
	svc := newIdleHostCalibrationService(repo, runner, nil, WithHWMode(domaintranscoding.HWModeVAAPI))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.Zero(t, runner.calls.Load(), "mode pin matching the cache must still skip the bench")
	assert.Equal(t, "vaapi", svc.Current().HWBackend, "persisted pinned backend kept")
}

func TestCalibration_EnsureCalibrated_ModeOff_RebenchesSoftwareOnly(t *testing.T) {
	// Mode=off but the persisted cache carries a HW backend → re-bench, and the
	// bench must be told to disable HW so the host calibrates software-only.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc", CPUSignature: hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, CPUSignature: hostSig},
	}
	svc := newIdleHostCalibrationService(repo, runner, nil, WithHWMode(domaintranscoding.HWModeOff))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.EqualValues(t, 1, runner.calls.Load(), "mode=off with persisted HW must re-bench software-only")
	assert.Empty(t, svc.Current().HWBackend, "software-only after off")
	if p := runner.lastPref.Load(); assert.NotNil(t, p) {
		assert.True(t, p.DisableHW, "the bench must be told to skip HW")
	}
}

func TestCalibration_SetHWMode_OffThenAuto(t *testing.T) {
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc", CPUSignature: hostSig,
		},
	}
	svc := newIdleHostCalibrationService(repo, runner, nil) // defaults to auto
	require.Equal(t, domaintranscoding.HWModeAuto, svc.HWMode())

	require.NoError(t, svc.SetHWMode(context.Background(), domaintranscoding.HWModeOff))
	assert.Equal(t, domaintranscoding.HWModeOff, svc.HWMode())
	if p := runner.lastPref.Load(); assert.NotNil(t, p) {
		assert.True(t, p.DisableHW, "off must disable HW in the bench preference")
	}

	require.NoError(t, svc.SetHWMode(context.Background(), domaintranscoding.HWModeNVENC))
	assert.Equal(t, domaintranscoding.HWModeNVENC, svc.HWMode())
	if p := runner.lastPref.Load(); assert.NotNil(t, p) {
		assert.Equal(t, "nvenc", p.Backend)
		assert.False(t, p.DisableHW)
	}
}

func TestCalibration_EnsureCalibrated_StaleCpuSig_Rebench(t *testing.T) {
	// Persisted capability with a DIFFERENT cpu signature → container
	// migrated, must rebench.
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080,
			CPUSignature:      "different-host-sig",
		},
	}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 720},
	}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.EqualValues(t, 1, runner.calls.Load())
	assert.Equal(t, 720, svc.Current().SoftwareMaxHeight)
}

func TestCalibration_Recalibrate_AlwaysRebench(t *testing.T) {
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080,
			CPUSignature:      hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 2160, CPUSignature: hostSig},
	}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background())) // 0 calls (cached)
	require.NoError(t, svc.Recalibrate(context.Background()))      // 1 call (forced)

	assert.EqualValues(t, 1, runner.calls.Load(), "Recalibrate must force the bench even when cache is valid")
	assert.Equal(t, 2160, svc.Current().SoftwareMaxHeight)
}

func TestCalibration_Recalibrate_RejectsConcurrent(t *testing.T) {
	repo := &fakeCapabilityRepo{}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080},
		delay:  100 * time.Millisecond, // long enough for the second call to hit while the first is running
	}
	svc := newIdleHostCalibrationService(repo, runner, nil)

	errCh := make(chan error, 2)
	go func() { errCh <- svc.Recalibrate(context.Background()) }()
	// Small race buffer so the goroutine acquires the lock first.
	time.Sleep(10 * time.Millisecond)
	errCh <- svc.Recalibrate(context.Background())

	var got []error
	for range 2 {
		got = append(got, <-errCh)
	}
	hasBusy := false
	hasOK := false
	for _, e := range got {
		switch {
		case errors.Is(e, ErrBenchmarkBusy):
			hasBusy = true
		case e == nil:
			hasOK = true
		}
	}
	assert.True(t, hasOK, "one of the two recalibrations must succeed")
	assert.True(t, hasBusy, "the other must return ErrBenchmarkBusy (no piling up)")
}

func TestCalibration_EnsureCalibrated_RepoFailure_StaysOnSafeFallback(t *testing.T) {
	repo := &fakeCapabilityRepo{getErr: errors.New("db unreachable")}
	runner := &fakeBenchmarkRunner{}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()), "repo errors must be non-fatal at boot")
	assert.Equal(t, 720, svc.Current().SoftwareMaxHeight, "stays on safe fallback when repo is down")
	assert.Zero(t, runner.calls.Load(), "no bench when we can't read repo (would lose the value anyway)")
}

func TestCalibration_BenchAndPersist_RunnerErrorPropagates(t *testing.T) {
	svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, &fakeBenchmarkRunner{err: errors.New("ffmpeg crash")}, nil)
	err := svc.Recalibrate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg crash")
}

// TestCalibration_Recalibrate_IdleGate locks the idle-gate: a manual
// recalibration is refused while transcodes are active (it would measure
// contended throughput and pin the cap too low), but first-boot calibration is
// never gated.
func TestCalibration_Recalibrate_IdleGate(t *testing.T) {
	newSvc := func(checker func() int) (*CalibrationService, *fakeBenchmarkRunner) {
		runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, CPUSignature: "sig",
		}}
		svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, runner, nil)
		if checker != nil {
			svc.SetActiveStreamChecker(checker)
		}
		return svc, runner
	}

	t.Run("refuses while transcodes active", func(t *testing.T) {
		svc, runner := newSvc(func() int { return 2 })
		err := svc.Recalibrate(context.Background())
		require.ErrorIs(t, err, ErrBenchmarkBusyTranscoding)
		assert.Equal(t, int64(0), runner.calls.Load(), "bench must NOT run while streams are active")
	})

	t.Run("proceeds when idle", func(t *testing.T) {
		svc, runner := newSvc(func() int { return 0 })
		require.NoError(t, svc.Recalibrate(context.Background()))
		assert.Equal(t, int64(1), runner.calls.Load(), "bench runs when no streams are active")
		assert.Equal(t, 1080, svc.Current().SoftwareMaxHeight)
	})

	t.Run("nil checker disables the gate", func(t *testing.T) {
		svc, runner := newSvc(nil)
		require.NoError(t, svc.Recalibrate(context.Background()))
		assert.Equal(t, int64(1), runner.calls.Load())
	})

	t.Run("first-boot EnsureCalibrated is never gated", func(t *testing.T) {
		svc, runner := newSvc(func() int { return 5 })
		require.NoError(t, svc.EnsureCalibrated(context.Background()))
		assert.Equal(t, int64(1), runner.calls.Load(), "first-boot bench runs regardless of active streams (no streams at boot)")
	})
}

// IsBenchmarking must report true for exactly the duration of a bench run, so
// background work (intro detection) can yield the CPU to the calibration.
func TestCalibration_IsBenchmarking_TrueDuringBench(t *testing.T) {
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, CPUSignature: "sig"},
		delay:  200 * time.Millisecond,
	}
	svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, runner, nil)

	assert.False(t, svc.IsBenchmarking(), "not benchmarking at rest")

	done := make(chan error, 1)
	go func() { done <- svc.Recalibrate(context.Background()) }()

	require.Eventually(t, svc.IsBenchmarking, time.Second, 5*time.Millisecond,
		"IsBenchmarking must be true while the bench is in flight")
	require.NoError(t, <-done)
	assert.False(t, svc.IsBenchmarking(), "cleared once the bench finishes")
}

// hostCPUSig wraps sysinfo.CPUSignature with a skip when /proc/cpuinfo
// is unreadable. Without a stable host signature the cache-hit and
// rebench tests can't run meaningfully.
func hostCPUSig(t *testing.T) string {
	t.Helper()
	sig := cpuSigForTest()
	if sig == "" {
		t.Skip("/proc/cpuinfo not readable (likely non-Linux CI runner)")
	}
	return sig
}

// ============================================================================
// HW demotion is on a clock (ReprobeDemotedHW)
// ============================================================================

// demotedService: HW switched off by a failure demotedFor ago, backend still
// advertised by the host.
func demotedService(t *testing.T, runner *fakeBenchmarkRunner, demotedFor time.Duration) *CalibrationService {
	t.Helper()
	svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, runner, nil)
	demoted := domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1080,
		HWBackend:         "", // switched off by the failure
		HWMaxHeight:       0,
		DetectedBackends:  []string{"vaapi"},
		HWDemotedAt:       time.Now().UTC().Add(-demotedFor),
	}
	svc.current.Store(&demoted)
	return svc
}

// The production incident: a demotion past its TTL must not keep the host on
// CPU once the GPU answers again.
func TestCalibration_ReprobeDemotedHW_ExpiredAndUsable_Restores(t *testing.T) {
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080,
			HWMaxHeight:       2160,
			HWBackend:         "vaapi",
			CPUSignature:      "fake-sig",
		},
	}
	svc := demotedService(t, runner, HWDemotionTTL+time.Minute)

	assert.True(t, svc.ReprobeDemotedHW(context.Background()))
	assert.Equal(t, "vaapi", svc.Current().HWBackend, "hardware must come back")
	assert.EqualValues(t, 1, runner.calls.Load(), "restoring goes through a real re-bench, not the pre-demotion values")
}

// Inside the TTL nothing is probed - what keeps a broken GPU off the tick.
func TestCalibration_ReprobeDemotedHW_WithinTTL_DoesNothing(t *testing.T) {
	runner := &fakeBenchmarkRunner{}
	svc := demotedService(t, runner, time.Minute)

	assert.False(t, svc.ReprobeDemotedHW(context.Background()))
	assert.Empty(t, svc.Current().HWBackend)
	assert.EqualValues(t, 0, runner.calls.Load(), "no bench inside the TTL")
}

// A dead backend stays demoted: the timer never forgives, only a probe does.
func TestCalibration_ReprobeDemotedHW_StillDead_StaysSoftware(t *testing.T) {
	runner := &fakeBenchmarkRunner{hwUnusable: true}
	svc := demotedService(t, runner, HWDemotionTTL+time.Minute)

	assert.False(t, svc.ReprobeDemotedHW(context.Background()))
	assert.Empty(t, svc.Current().HWBackend)
	assert.EqualValues(t, 0, runner.calls.Load(), "a failed probe must not trigger a re-bench")
}

// ...and the clock restarts: one probe per TTL, not one per tick.
func TestCalibration_ReprobeDemotedHW_StillDead_ResetsTheClock(t *testing.T) {
	runner := &fakeBenchmarkRunner{hwUnusable: true}
	svc := demotedService(t, runner, HWDemotionTTL+time.Minute)
	before := svc.Current().HWDemotedAt

	require.False(t, svc.ReprobeDemotedHW(context.Background()))

	after := svc.Current().HWDemotedAt
	assert.True(t, after.After(before), "the demotion clock must restart after a failed probe")
	assert.False(t, svc.ReprobeDemotedHW(context.Background()), "and the next call is inside the fresh TTL")
}

// An operator who turned hardware off must never have it probed back on.
func TestCalibration_ReprobeDemotedHW_HWModeOff_NeverProbes(t *testing.T) {
	runner := &fakeBenchmarkRunner{}
	svc := demotedService(t, runner, HWDemotionTTL+time.Minute)
	svc.mode = domaintranscoding.HWModeOff

	assert.False(t, svc.ReprobeDemotedHW(context.Background()))
	assert.EqualValues(t, 0, runner.calls.Load())
}

// A host that was never demoted has nothing to reprobe.
func TestCalibration_ReprobeDemotedHW_NeverDemoted_DoesNothing(t *testing.T) {
	runner := &fakeBenchmarkRunner{}
	svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, runner, nil)
	healthy := domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, HWBackend: "vaapi", HWMaxHeight: 2160}
	svc.current.Store(&healthy)

	assert.False(t, svc.ReprobeDemotedHW(context.Background()))
	assert.EqualValues(t, 0, runner.calls.Load())
}

// The stamp puts a demotion on the clock, so it must be written exactly when a
// failure switches the backend off.
func TestCalibration_DemoteCapability_StampsWhenHWGoesOff(t *testing.T) {
	repo := &fakeCapabilityRepo{}
	svc := newIdleHostCalibrationService(repo, &fakeBenchmarkRunner{}, nil)
	live := domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1080, HWBackend: "vaapi", HWMaxHeight: 2160,
		HWCodecs: []string{"h264"}, HWDevice: "/dev/dri/renderD128",
	}
	svc.current.Store(&live)

	require.False(t, svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWInit, "h264"), "first strike")
	require.True(t, svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWInit, "h264"))

	assert.Empty(t, svc.Current().HWBackend)
	assert.False(t, svc.Current().HWDemotedAt.IsZero(), "a demotion that kills HW must be stamped, or it can never expire")
}

// A feature trim leaves hardware working, so it is not on the clock.
func TestCalibration_DemoteCapability_FeatureTrim_NotStamped(t *testing.T) {
	svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, &fakeBenchmarkRunner{}, nil)
	live := domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1080, HWBackend: "vaapi", HWMaxHeight: 2160,
		HWFeatures: []string{"temporal_aq"},
	}
	svc.current.Store(&live)

	require.True(t, svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWUnsupportedOption, "h264"))

	assert.Equal(t, "vaapi", svc.Current().HWBackend, "the backend keeps working")
	assert.True(t, svc.Current().HWDemotedAt.IsZero(), "nothing was switched off, so nothing is on the clock")
}

// newIdleHostCalibrationService builds the service the way every existing test
// expects it: on a host that is NOT busy. The production default samples the
// real /proc/stat for a second before each bench; here that would only slow
// the suite down and make the contention verdict depend on the CI runner.
func newIdleHostCalibrationService(repo domaintranscoding.CapabilityRepo, runner domaintranscoding.BenchmarkRunner, logger *slog.Logger, opts ...CalibrationOption) *CalibrationService {
	return NewCalibrationService(repo, runner, logger, append([]CalibrationOption{WithHostLoadProbe(func() float64 { return 0 })}, opts...)...)
}

// ============================================================================
// A benchmark on a busy host is provisional (BenchContended)
// ============================================================================

// The production incident: the first boot of a 4K-capable host ran its
// benchmark while a library import pinned every core - 720p@5fps, "no usable
// GPU", capacity 1 - and that result was persisted as the truth about the box.
func TestCalibration_BenchOnBusyHost_IsFlaggedContended(t *testing.T) {
	repo := &fakeCapabilityRepo{}
	runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 720, CPUSignature: "fake-sig"}}
	svc := NewCalibrationService(repo, runner, nil, WithHostLoadProbe(func() float64 { return 87 }))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	cur := svc.Current()
	assert.True(t, cur.BenchContended, "a bench started on a busy host must be marked provisional")
	assert.Equal(t, 87, cur.BenchHostBusyPercent, "the evidence travels with the verdict")
	persisted, found, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, persisted.BenchContended, "the flag must survive a restart, or the next boot trusts the bad bench")
}

func TestCalibration_BenchOnIdleHost_IsNotContended(t *testing.T) {
	repo := &fakeCapabilityRepo{}
	runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, CPUSignature: "fake-sig"}}
	svc := NewCalibrationService(repo, runner, nil, WithHostLoadProbe(func() float64 { return 12 }))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))
	assert.False(t, svc.Current().BenchContended)
	assert.Equal(t, 12, svc.Current().BenchHostBusyPercent)
}

// contendedService: a persisted-then-loaded provisional capability, the host
// load answered by probe.
func contendedService(t *testing.T, runner *fakeBenchmarkRunner, probe func() float64) *CalibrationService {
	t.Helper()
	svc := NewCalibrationService(&fakeCapabilityRepo{}, runner, nil, WithHostLoadProbe(probe))
	cur := domaintranscoding.EncodeCapability{SoftwareMaxHeight: 720, CPUSignature: "fake-sig", BenchContended: true, BenchHostBusyPercent: 91}
	svc.current.Store(&cur)
	return svc
}

func TestCalibration_RebenchIfContended_IdleHost_Rebenches(t *testing.T) {
	runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1440, HWMaxHeight: 2160, HWBackend: "vaapi", CPUSignature: "fake-sig"}}
	svc := contendedService(t, runner, func() float64 { return 3 })

	assert.True(t, svc.RebenchIfContended(context.Background()))
	assert.EqualValues(t, 1, runner.calls.Load(), "an idle host re-benches the provisional result")
	cur := svc.Current()
	assert.False(t, cur.BenchContended, "the fresh bench replaces the provisional one")
	assert.Equal(t, 1440, cur.SoftwareMaxHeight)
	assert.Equal(t, "vaapi", cur.HWBackend, "the GPU the busy bench missed comes back")
}

func TestCalibration_RebenchIfContended_HostStillBusy_Waits(t *testing.T) {
	runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1440, CPUSignature: "fake-sig"}}
	svc := contendedService(t, runner, func() float64 { return 64 })

	assert.False(t, svc.RebenchIfContended(context.Background()))
	assert.EqualValues(t, 0, runner.calls.Load(), "a busy host would only produce another contended bench")
	assert.True(t, svc.Current().BenchContended)
}

func TestCalibration_RebenchIfContended_NotContended_NoOp(t *testing.T) {
	runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1440, CPUSignature: "fake-sig"}}
	svc := newIdleHostCalibrationService(&fakeCapabilityRepo{}, runner, nil)
	cur := domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, CPUSignature: "fake-sig"}
	svc.current.Store(&cur)

	assert.False(t, svc.RebenchIfContended(context.Background()))
	assert.EqualValues(t, 0, runner.calls.Load(), "a trusted calibration is never re-benched behind the operator's back")
}

func TestCalibration_RebenchIfContended_StreamsActive_Waits(t *testing.T) {
	runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1440, CPUSignature: "fake-sig"}}
	svc := contendedService(t, runner, func() float64 { return 3 })
	svc.SetActiveStreamChecker(func() int { return 2 })

	assert.False(t, svc.RebenchIfContended(context.Background()))
	assert.EqualValues(t, 0, runner.calls.Load(), "never bench over real viewers")
}

// A host that looks idle for the pre-check but is busy again by the time the
// bench starts yields another contended result: the retry must back off rather
// than burn ~3 min of full CPU every tick.
func TestCalibration_RebenchIfContended_ContendedAgain_BacksOff(t *testing.T) {
	runner := &fakeBenchmarkRunner{result: domaintranscoding.EncodeCapability{SoftwareMaxHeight: 720, CPUSignature: "fake-sig"}}
	var probes atomic.Int64
	svc := contendedService(t, runner, func() float64 {
		// 1st probe: the idle pre-check passes; 2nd probe (inside the bench): busy.
		if probes.Add(1) == 1 {
			return 0
		}
		return 95
	})

	assert.False(t, svc.RebenchIfContended(context.Background()), "the bench ran but came back contended")
	assert.EqualValues(t, 1, runner.calls.Load())
	assert.True(t, svc.Current().BenchContended)

	assert.False(t, svc.RebenchIfContended(context.Background()))
	assert.EqualValues(t, 1, runner.calls.Load(), "inside the backoff window no second bench runs, whatever the probe says")
}

// A persisted HW capability benched before a new probe existed is re-benched
// once at boot, so the probe's field is populated; a software-only row is not.
func TestCalibration_EnsureCalibrated_OldProbeVersion_Rebenches(t *testing.T) {
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "vaapi", CPUSignature: hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{
		result: domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "vaapi", CPUSignature: hostSig,
			HWHDR10Encode: true, ProbeVersion: domaintranscoding.CapabilityProbeVersion,
		},
	}
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))
	assert.EqualValues(t, 1, runner.calls.Load(), "old probe version with a HW backend must re-bench")
	assert.True(t, svc.Current().HWHDR10Encode)

	softwareOnly := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, CPUSignature: hostSig},
	}
	runner2 := &fakeBenchmarkRunner{}
	require.NoError(t, newIdleHostCalibrationService(softwareOnly, runner2, nil).EnsureCalibrated(context.Background()))
	assert.Zero(t, runner2.calls.Load(), "no HW backend: nothing new to probe")
}
