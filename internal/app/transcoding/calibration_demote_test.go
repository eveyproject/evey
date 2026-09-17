package transcoding

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

func seedHWService(t *testing.T) (*CalibrationService, *fakeCapabilityRepo) {
	t.Helper()
	// The default fake runner CONFIRMS the host on every probe; the tests that
	// exercise a real host demotion make the proof fail explicitly.
	svc, repo, _ := seedHWServiceWith(t, &fakeBenchmarkRunner{})
	return svc, repo
}

func seedHWServiceWith(t *testing.T, runner *fakeBenchmarkRunner) (*CalibrationService, *fakeCapabilityRepo, *fakeBenchmarkRunner) {
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
	runner.result = hwCap
	svc := newIdleHostCalibrationService(repo, runner, nil)
	require.NoError(t, svc.Recalibrate(context.Background())) // current = hwCap
	return svc, repo, runner
}

func TestDemoteCapability_UnsupportedOption_NarrowsToReconfirmedFeatures_Persists(t *testing.T) {
	svc, repo, _ := seedHWServiceWith(t, &fakeBenchmarkRunner{refusedFeatures: []string{"temporal_aq"}})
	ctx := context.Background()

	ok := svc.DemoteCapability(ctx, domaintranscoding.FailureHWUnsupportedOption, "h264")
	assert.True(t, ok, "a feature the host itself refuses must change the capability")

	cur := svc.Current()
	assert.Equal(t, []string{"rc_lookahead"}, cur.HWFeatures, "only the refused feature is dropped, the re-confirmed one stays")
	assert.Equal(t, "nvenc", cur.HWBackend, "HW backend survives an option reject")
	assert.Equal(t, 2160, cur.HWMaxHeight)

	stored, found, _ := repo.Get(ctx)
	require.True(t, found, "deterministic demotion must be persisted")
	assert.Equal(t, []string{"rc_lookahead"}, stored.HWFeatures)

	events := svc.RecentFallbacks()
	require.Len(t, events, 1)
	assert.Equal(t, "host_demoted", events[0].Verdict)

	// Debounce: a second identical failure re-confirms the remaining feature
	// and changes nothing.
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWUnsupportedOption, "h264"))
	assert.Equal(t, []string{"rc_lookahead"}, svc.Current().HWFeatures)
}

// The invariant this file exists for: a session failure is NOT a host verdict.
// When the media-independent probe passes, the host keeps every feature and
// its backend, whatever class the session reported; only the event is kept.
func TestDemoteCapability_HostReconfirmed_NoChange(t *testing.T) {
	for _, class := range []domaintranscoding.FailureClass{
		domaintranscoding.FailureHWUnsupportedOption,
		domaintranscoding.FailureHWEncoderMissing,
		domaintranscoding.FailureHWInit,
		domaintranscoding.FailureHWCorruptOutput,
	} {
		svc, repo, runner := seedHWServiceWith(t, &fakeBenchmarkRunner{})
		before := svc.Current()
		assert.False(t, svc.DemoteCapability(context.Background(), class, "hevc"), "class %s: host confirmed, no demotion", class)
		assert.Equal(t, before, svc.Current(), "class %s: capability untouched", class)
		assert.Positive(t, runner.proofCalls.Load(), "class %s: the proof must have been asked", class)
		stored, _, _ := repo.Get(context.Background())
		assert.Equal(t, before, stored, "class %s: the persisted row is the bench, untouched", class)
		events := svc.RecentFallbacks()
		require.Len(t, events, 1, "class %s", class)
		assert.Equal(t, "media_route", events[0].Verdict)
	}
}

// A host proof that just passed is not run again for the next failures of
// the same (backend, codec): a storm of sessions dying on one bad title costs
// one probe, not one per session queued behind demoteMu. The memo expires, and
// any demotion forgets it - the capability it described is gone.
func TestDemoteCapability_RecentProof_IsReused(t *testing.T) {
	ctx := context.Background()
	svc, _, runner := seedHWServiceWith(t, &fakeBenchmarkRunner{})

	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWInit, "hevc"))
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWCorruptOutput, "hevc"))
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWEncoderMissing, "hevc"))
	assert.EqualValues(t, 1, runner.proofCalls.Load(), "one encode proof serves every failure of the window")
	events := svc.RecentFallbacks()
	require.NotEmpty(t, events)
	assert.Equal(t, "media_route", events[0].Verdict, "the reused proof still records the failure as the media's")

	// Another codec, and the feature probe, are their own proofs.
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWInit, "h264"))
	assert.EqualValues(t, 2, runner.proofCalls.Load())
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWUnsupportedOption, "hevc"))
	featureProbe := runner.proofCalls.Load()
	assert.Greater(t, featureProbe, int64(2), "the feature proof runs once")
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWUnsupportedOption, "hevc"))
	assert.Equal(t, featureProbe, runner.proofCalls.Load(), "and is reused inside the window")

	// Expired: the host is asked again.
	svc.demoteMu.Lock()
	svc.hwProofOK[hwProofKey("nvenc", "hevc", "encode")] = time.Now().Add(-hwProofReuseWindow)
	svc.demoteMu.Unlock()
	assert.False(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWInit, "hevc"))
	assert.Equal(t, featureProbe+1, runner.proofCalls.Load(), "an expired proof is re-run")

	// A demotion forgets every proof: the next failure probes the new capability.
	// (The h264 proof is expired first: inside its window even a dead GPU is not
	// re-asked - that is the 30 s the memo trades for the queue.)
	runner.encodeUnusable = true
	svc.demoteMu.Lock()
	svc.hwProofOK[hwProofKey("nvenc", "h264", "encode")] = time.Now().Add(-hwProofReuseWindow)
	svc.demoteMu.Unlock()
	assert.True(t, svc.DemoteCapability(ctx, domaintranscoding.FailureHWEncoderMissing, "h264"), "a failed proof demotes")
	svc.demoteMu.Lock()
	assert.Empty(t, svc.hwProofOK, "no proof survives a capability change")
	svc.demoteMu.Unlock()
}

// A media/route incompatibility never asks the host anything: the title is
// the orchestrator's to send to software; here it is only recorded.
func TestDemoteCapability_RouteIncompatible_RecordsOnly(t *testing.T) {
	svc, _, runner := seedHWServiceWith(t, &fakeBenchmarkRunner{encodeUnusable: true})
	before := svc.Current()
	assert.False(t, svc.DemoteCapability(context.Background(), domaintranscoding.FailureRouteIncompatible, "h264"))
	assert.Equal(t, before, svc.Current())
	assert.Zero(t, runner.proofCalls.Load(), "no host probe for a media/route failure")
	events := svc.RecentFallbacks()
	require.Len(t, events, 1)
	assert.Equal(t, "route_incompatible", events[0].Class)
	assert.Equal(t, "media_route", events[0].Verdict)
}

func TestDemoteCapability_EncoderMissing_RemovesCodec(t *testing.T) {
	svc, _, _ := seedHWServiceWith(t, &fakeBenchmarkRunner{encodeUnusable: true})
	ok := svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWEncoderMissing, "hevc")
	assert.True(t, ok)
	cur := svc.Current()
	assert.NotContains(t, cur.HWCodecs, "hevc")
	assert.Contains(t, cur.HWCodecs, "h264", "other codecs keep HW")
	assert.Equal(t, "nvenc", cur.HWBackend)
}

func TestDemoteCapability_InitFailure_DisablesHW(t *testing.T) {
	svc, repo, _ := seedHWServiceWith(t, &fakeBenchmarkRunner{encodeUnusable: true})
	assert.False(t, svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWInit, "h264"), "one missed init is a strike, not a verdict")
	assert.Equal(t, "nvenc", svc.Current().HWBackend)
	events := svc.RecentFallbacks()
	require.Len(t, events, 1, "the strike is still visible on the dashboard")
	assert.Equal(t, "strike", events[0].Verdict)
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
	svc, _, _ := seedHWServiceWith(t, &fakeBenchmarkRunner{refusedFeatures: []string{"temporal_aq", "rc_lookahead"}})
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			svc.DemoteCapability(context.Background(), domaintranscoding.FailureHWUnsupportedOption, "h264")
		})
	}
	wg.Wait()
	// The storm of concurrent failures from one bad option must collapse to a
	// single capability change: one host_demoted event, and the losers of the
	// race - whose re-probe of the narrowed set passes - one media_route event.
	events := svc.RecentFallbacks()
	require.NotEmpty(t, events)
	assert.LessOrEqual(t, len(events), 2)
	assert.Equal(t, "host_demoted", events[0].Verdict)
	assert.Empty(t, svc.Current().HWFeatures)
}

// The video admission cost model prices streams on bench data; a capability
// change that does not re-arm it leaves admission running on a dead snapshot
// (seen live: an NVENC init failure demoted hw and nothing repriced). Every
// runtime mutation - bench completion and demotion alike - must notify.
func TestCalibration_OnCapabilityChange_FiresOnBenchAndDemote(t *testing.T) {
	svc, _, _ := seedHWServiceWith(t, &fakeBenchmarkRunner{refusedFeatures: []string{"temporal_aq", "rc_lookahead"}})
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

// A failed hardware session reports the exact route (backend, encoder, cost
// class), not just its codec: the fallback record names what failed, so the
// admin panel can tell a bridge failure from a zero-copy one (ADR-0030, 7).
// The verdict rules themselves are unchanged (still keyed on class + codec).
func TestDemoteRoute_RecordsTheExactRouteOnTheFallbackEvent(t *testing.T) {
	svc, _ := seedHWService(t)
	fp := DecisionFingerprint{VideoCodec: "hevc", HWAccel: "nvenc", HDRTonemap: true,
		Profiles: []ProfileFingerprint{{Name: "1080p", Width: 1920, Height: 1080}}}
	report := hwFailure(domaintranscoding.FailureRouteIncompatible, fp)
	assert.Equal(t, RouteCostKeyOf(fp), report.Route)
	assert.Equal(t, domaintranscoding.RouteCostBridge, report.Route.Route, "HEVC tone-mapped on NVENC without a GPU tone-map = the bridge")
	assert.Equal(t, "hevc", report.Codec)

	assert.False(t, svc.DemoteRoute(context.Background(), report), "a media/route incompatibility never touches the host")
	events := svc.RecentFallbacks()
	require.Len(t, events, 1)
	assert.Equal(t, "nvenc", events[0].Backend)
	assert.Equal(t, "bridge", events[0].Route)
	assert.Equal(t, "hevc", events[0].Codec)
	assert.Equal(t, fallbackVerdictMediaRoute, events[0].Verdict)
}
