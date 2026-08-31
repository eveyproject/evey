package transcoding

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

func seedHWService(t *testing.T) (*CalibrationService, *fakeCapabilityRepo) {
	t.Helper()
	hwCap := domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1080,
		HWMaxHeight:       2160,
		HWBackend:         "nvenc",
		HWCodecs:          []string{"h264", "hevc"},
		HWFeatures:        []string{"temporal_aq", "rc_lookahead"},
		CPUSignature:      "sig",
	}
	repo := &fakeCapabilityRepo{}
	svc := newIdleHostCalibrationService(repo, &fakeBenchmarkRunner{result: hwCap}, nil)
	require.NoError(t, svc.Recalibrate(context.Background())) // current = hwCap
	return svc, repo
}

func TestDemoteCapability_UnsupportedOption_DropsTuning_Persists(t *testing.T) {
	svc, repo := seedHWService(t)
	ctx := context.Background()

	ok := svc.DemoteCapability(ctx, domaintranscoding.FailureHWUnsupportedOption, "h264")
	assert.True(t, ok, "first demote must change the capability")

	cur := svc.Current()
	assert.Empty(t, cur.HWFeatures, "tuning dropped")
	assert.Equal(t, "nvenc", cur.HWBackend, "HW backend survives an option reject")
	assert.Equal(t, 2160, cur.HWMaxHeight)

	stored, found, _ := repo.Get(ctx)
	require.True(t, found, "deterministic demotion must be persisted")
	assert.Empty(t, stored.HWFeatures)

	assert.Len(t, svc.RecentFallbacks(), 1)

	// Debounce: a second identical failure is a no-op (HWFeatures already empty).
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWUnsupportedOption, "h264"))
	assert.Len(t, svc.RecentFallbacks(), 1, "no duplicate fallback record")
}

func TestDemoteCapability_EncoderMissing_RemovesCodec(t *testing.T) {
	svc, _ := seedHWService(t)
	ok := svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWEncoderMissing, "hevc")
	assert.True(t, ok)
	cur := svc.Current()
	assert.NotContains(t, cur.HWCodecs, "hevc")
	assert.Contains(t, cur.HWCodecs, "h264", "other codecs keep HW")
	assert.Equal(t, "nvenc", cur.HWBackend)
}

func TestDemoteCapability_InitFailure_DisablesHW(t *testing.T) {
	svc, repo := seedHWService(t)
	ok := svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWInit, "h264")
	assert.True(t, ok)
	cur := svc.Current()
	assert.Empty(t, cur.HWBackend)
	assert.Zero(t, cur.HWMaxHeight)
	assert.Equal(t, 1080, cur.SoftwareMaxHeight, "software ceiling survives")
	stored, _, _ := repo.Get(context.Background())
	assert.Empty(t, stored.HWBackend)
}

func TestDemoteCapability_TransientAndNonHW_AreNoOps(t *testing.T) {
	for _, class := range []domaintranscoding.FailureClass{
		domaintranscoding.FailureHWDeviceMemory, // transient → no persistent demote
		domaintranscoding.FailureTransient,
		domaintranscoding.FailureCancelled,
		domaintranscoding.FailureUnknown,
		domaintranscoding.FailureNone,
	} {
		svc, _ := seedHWService(t)
		if svc.DemoteCapability(context.Background(), class, "h264") {
			t.Errorf("class %q must not demote the persistent capability", class)
		}
		if svc.Current().HWBackend != "nvenc" {
			t.Errorf("class %q left the capability changed", class)
		}
		if len(svc.RecentFallbacks()) != 0 {
			t.Errorf("class %q must not record a fallback", class)
		}
	}
}

func TestDemoteCapability_ConcurrentFailures_Debounce(t *testing.T) {
	svc, _ := seedHWService(t)
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWUnsupportedOption, "h264")
		})
	}
	wg.Wait()
	// The storm of concurrent failures from one bad option must collapse to a
	// single capability change + a single recorded fallback.
	assert.Len(t, svc.RecentFallbacks(), 1)
	assert.Empty(t, svc.Current().HWFeatures)
}

// The video admission cost model prices streams on bench data; a capability
// change that does not re-arm it leaves admission running on a dead snapshot
// (seen live: an NVENC init failure demoted hw and nothing repriced). Every
// runtime mutation - bench completion and demotion alike - must notify.
func TestCalibration_OnCapabilityChange_FiresOnBenchAndDemote(t *testing.T) {
	svc, _ := seedHWService(t)
	var fired int
	svc.OnCapabilityChange(func() { fired++ })

	require.NoError(t, svc.Recalibrate(context.Background()))
	assert.Equal(t, 1, fired, "bench completion must notify")

	require.True(t, svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWUnsupportedOption, "h264"))
	assert.Equal(t, 2, fired, "a runtime demotion must notify")

	// A demote that changes nothing must not re-fire (no notify churn).
	svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWUnsupportedOption, "h264")
	assert.Equal(t, 2, fired)
}
