package transcoding

import (
	"context"
	"errors"
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
}

func (r *fakeBenchmarkRunner) Run(ctx context.Context) (domaintranscoding.EncodeCapability, error) {
	r.calls.Add(1)
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
	svc := NewCalibrationService(&fakeCapabilityRepo{}, &fakeBenchmarkRunner{}, nil)
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
	svc := NewCalibrationService(repo, runner, nil)
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
	// check passes — the test relies on /proc/cpuinfo being stable.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1440,
			HWMaxHeight:       0,
			CPUSignature:      hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{}
	svc := NewCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.Zero(t, runner.calls.Load(), "matching CPU sig must skip the benchmark")
	assert.Equal(t, 1440, svc.Current().SoftwareMaxHeight, "Current() must reflect the persisted cap")
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
		},
	}
	runner := &fakeBenchmarkRunner{hwUnusable: true} // HW dead on this host
	svc := NewCalibrationService(repo, runner, nil)
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
			SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc", CPUSignature: hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{} // hwUnusable=false → usable here
	svc := NewCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	cur := svc.Current()
	assert.Equal(t, 2160, cur.HWMaxHeight, "usable HW kept")
	assert.Equal(t, "nvenc", cur.HWBackend, "usable HW backend kept")
}

func TestCalibration_EnsureCalibrated_PinMismatch_Rebench(t *testing.T) {
	// Matching CPU sig, but this instance is pinned to a DIFFERENT backend than
	// the persisted one (a sibling container on another GPU path wrote the cache
	// to the shared DB). The pin must win: re-bench instead of trusting the cache.
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
	svc := NewCalibrationService(repo, runner, nil, WithPinnedBackend("vaapi"))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.EqualValues(t, 1, runner.calls.Load(), "pin mismatch must force a fresh bench, not trust the sibling's nvenc cache")
	assert.Equal(t, "vaapi", svc.Current().HWBackend, "Current() must reflect the pinned backend after re-bench")
}

func TestCalibration_EnsureCalibrated_PinMatch_SkipsBench(t *testing.T) {
	// Pinned to the SAME backend the cache holds → the cache is trusted as usual.
	hostSig := hostCPUSig(t)
	repo := &fakeCapabilityRepo{
		stored: &domaintranscoding.EncodeCapability{
			SoftwareMaxHeight: 1080, HWMaxHeight: 1080, HWBackend: "vaapi", CPUSignature: hostSig,
		},
	}
	runner := &fakeBenchmarkRunner{} // hwUnusable=false → usable here
	svc := NewCalibrationService(repo, runner, nil, WithPinnedBackend("vaapi"))
	require.NoError(t, svc.EnsureCalibrated(context.Background()))

	assert.Zero(t, runner.calls.Load(), "pin matching the cache must still skip the bench")
	assert.Equal(t, "vaapi", svc.Current().HWBackend, "persisted pinned backend kept")
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
	svc := NewCalibrationService(repo, runner, nil)
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
	svc := NewCalibrationService(repo, runner, nil)
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
	svc := NewCalibrationService(repo, runner, nil)

	errCh := make(chan error, 2)
	go func() { errCh <- svc.Recalibrate(context.Background()) }()
	// Small race buffer so the goroutine acquires the lock first.
	time.Sleep(10 * time.Millisecond)
	errCh <- svc.Recalibrate(context.Background())

	var got []error
	for i := 0; i < 2; i++ {
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
	svc := NewCalibrationService(repo, runner, nil)
	require.NoError(t, svc.EnsureCalibrated(context.Background()), "repo errors must be non-fatal at boot")
	assert.Equal(t, 720, svc.Current().SoftwareMaxHeight, "stays on safe fallback when repo is down")
	assert.Zero(t, runner.calls.Load(), "no bench when we can't read repo (would lose the value anyway)")
}

func TestCalibration_BenchAndPersist_RunnerErrorPropagates(t *testing.T) {
	svc := NewCalibrationService(&fakeCapabilityRepo{}, &fakeBenchmarkRunner{err: errors.New("ffmpeg crash")}, nil)
	err := svc.Recalibrate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg crash")
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
