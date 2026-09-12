package transcoding

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// Close must be safe to call more than once: a second Close (e.g. defer +
// explicit shutdown, or two lifecycle owners) must not panic on a re-close
// of stopGC. The sync.Once guard makes the second call a no-op.
func TestOrchestrator_CloseIdempotent(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())

	require.NotPanics(t, func() {
		o.Close()
		o.Close()
	})
}

// A session whose ffmpeg exits with a HW-capability failure must drive the
// level-2 demote sink with the classified failure + the output codec.
func TestOrchestrator_HWCapabilityFailure_CallsDemoteSink(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.failOnExit = transcoding.FailureHWUnsupportedOption
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	type call struct {
		class transcoding.FailureClass
		codec string
	}
	got := make(chan call, 4)
	o.SetHWDemoteSink(func(_ context.Context, class transcoding.FailureClass, codec string) {
		got <- call{class, codec}
	})

	fp := minimalFingerprint(t)
	fp.VideoCodec = "h264"
	fp.HWAccel = "vaapi" // a HW-capability failure only demotes a HW session (usedHW guard)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	select {
	case c := <-got:
		assert.Equal(t, transcoding.FailureHWUnsupportedOption, c.class)
		assert.Equal(t, "h264", c.codec)
	case <-time.After(2 * time.Second):
		t.Fatal("demote sink was not called on a HW-capability failure")
	}
}

// A HW session that exits on its own without writing a single segment is a
// silent HW death the stderr classifier can't see (a driver crash / TDR GPU
// reset on WSL d3d12, or an exit-0 "Nothing was written"). The orchestrator
// must still demote to software (hw_init ⇒ disable HW + persist) so the
// client's retry plays on CPU instead of looping the broken HW path.
func TestOrchestrator_HWSessionNoSegments_DemotesToSoftware(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.failOnExit = transcoding.FailureUnknown // segfault-like: exits, no usable stderr
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	got := make(chan transcoding.FailureClass, 4)
	o.SetHWDemoteSink(func(_ context.Context, class transcoding.FailureClass, _ string) {
		got <- class
	})

	fp := minimalFingerprint(t)
	fp.HWAccel = "vaapi" // HW was selected for this session
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	select {
	case class := <-got:
		assert.Equal(t, transcoding.FailureHWInit, class,
			"a zero-output HW session must demote as hw_init (disable HW + persist)")
	case <-time.After(2 * time.Second):
		t.Fatal("demote sink was not called for a zero-output HW session")
	}
}

// The same zero-output exit on a SOFTWARE session must NOT demote: there is no
// HW to strip, and an empty software session is a media problem, not a HW one.
func TestOrchestrator_SoftwareSessionNoSegments_DoesNotDemote(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.failOnExit = transcoding.FailureUnknown
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	called := make(chan struct{}, 1)
	o.SetHWDemoteSink(func(_ context.Context, _ transcoding.FailureClass, _ string) {
		called <- struct{}{}
	})

	fp := minimalFingerprint(t) // HWAccel empty → software
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	select {
	case <-called:
		t.Fatal("demote sink fired on a software session")
	case <-time.After(300 * time.Millisecond):
		// expected: no demotion (usedHW=false)
	}
}

// A HW session that hangs producing nothing must be fast-failed by the
// watchdog: no init within the (tight, test) window → demote to software,
// without waiting for the full segment timeout. The fake handle blocks (never
// exits), simulating a wedged GPU that produces no output.
func TestOrchestrator_HWWatchdog_StuckHW_Demotes(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess() // failOnExit unset → handle blocks (ffmpeg "hangs")
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, OrchestratorOptions{
		LookAheadSegments:     30,
		SegmentSeconds:        3.0,
		HWFirstSegmentTimeout: 50 * time.Millisecond, // tight for the test
	}, discardLogger())
	defer o.Close()

	got := make(chan transcoding.FailureClass, 4)
	o.SetHWDemoteSink(func(_ context.Context, class transcoding.FailureClass, _ string) { got <- class })

	fp := minimalFingerprint(t)
	fp.HWAccel = "vaapi"
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	select {
	case class := <-got:
		assert.Equal(t, transcoding.FailureHWInit, class, "a stuck HW session (no init) must be watchdog-demoted to software")
	case <-time.After(2 * time.Second):
		t.Fatal("HW watchdog did not fire for a session that produced no init")
	}
}

// The watchdog is HW-only: a SOFTWARE session that produces nothing (a media
// problem, slow host) must NOT be killed/demoted by it - there's no HW to strip
// and software is already the floor.
func TestOrchestrator_HWWatchdog_SoftwareSession_NoDemote(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess() // blocks
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, OrchestratorOptions{
		LookAheadSegments:     30,
		SegmentSeconds:        3.0,
		HWFirstSegmentTimeout: 50 * time.Millisecond,
	}, discardLogger())
	defer o.Close()

	called := make(chan struct{}, 1)
	o.SetHWDemoteSink(func(_ context.Context, _ transcoding.FailureClass, _ string) { called <- struct{}{} })

	fp := minimalFingerprint(t) // HWAccel empty → software
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	select {
	case <-called:
		t.Fatal("HW watchdog fired on a software session")
	case <-time.After(300 * time.Millisecond): // well past the 50ms window
		// expected: no demotion
	}
}

// hwSessionAlive is the watchdog's safety signal: a written (non-empty) init
// segment must count as "alive" so a slow-but-working GPU is never killed.
func TestOrchestrator_hwSessionAlive_InitCountsAsAlive(t *testing.T) {
	cache, _ := newTestCache(t)
	o := NewSegmentOrchestrator(cache, newFakeProcess(), stubSource{}, testOpts(), discardLogger())
	defer o.Close()

	hash := "watchdog-alive-hash"
	assert.False(t, o.hwSessionAlive(hash, 0, true), "no output yet → not alive")

	dir, err := cache.DirFor(hash)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_0_init.mp4"), fmp4Init(), 0o644))

	assert.True(t, o.hwSessionAlive(hash, 0, true), "a written init segment must count as a live HW session")
}

// A natural exit with a non-HW class (e.g. cancellation) must NOT demote -
// exercises the IsHWCapability filter on a closed Done() channel.
func TestOrchestrator_NonHWExit_DoesNotDemote(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.failOnExit = transcoding.FailureCancelled // natural exit, not a HW fault
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	called := make(chan struct{}, 1)
	o.SetHWDemoteSink(func(_ context.Context, _ transcoding.FailureClass, _ string) {
		called <- struct{}{}
	})

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	select {
	case <-called:
		t.Fatal("demote sink fired on a non-HW exit")
	case <-time.After(300 * time.Millisecond):
		// expected: no demotion
	}
}

// The per-title half of the fallback ladder: every deterministic HW failure
// blocks the TITLE's GPU route (the client's retry then resolves to the CPU),
// and a media/route incompatibility still reaches the sink - which records it
// and never touches the host.
func TestOrchestrator_HWFailure_BlocksTitleRouteBeforeAnyHostVerdict(t *testing.T) {
	for _, class := range []transcoding.FailureClass{
		transcoding.FailureRouteIncompatible,
		transcoding.FailureHWUnsupportedOption,
		transcoding.FailureHWInit,
	} {
		t.Run(string(class), func(t *testing.T) {
			cache, _ := newTestCache(t)
			proc := newFakeProcess()
			proc.failOnExit = class
			src := stubSource{path: "/media/test.mkv"}
			o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger()).
				WithHWRouteBlocklist(NewHWRouteBlocklist(time.Hour, 16))
			defer o.Close()
			got := make(chan transcoding.FailureClass, 4)
			o.SetHWDemoteSink(func(_ context.Context, c transcoding.FailureClass, _ string) { got <- c })

			fp := minimalFingerprint(t)
			fp.VideoCodec = "h264"
			fp.HWAccel = "nvenc"
			require.False(t, o.HWRouteBlocked(fp.MediaID))
			require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

			select {
			case c := <-got:
				assert.Equal(t, class, c, "the sink hears the class as classified")
			case <-time.After(2 * time.Second):
				t.Fatal("sink not called")
			}
			assert.True(t, o.HWRouteBlocked(fp.MediaID), "the title's GPU route is blocked at once")
			entries := o.HWRouteBlocks()
			require.Len(t, entries, 1)
			assert.Equal(t, string(class), entries[0].Reason)

			o.ClearHWRouteBlocks()
			assert.False(t, o.HWRouteBlocked(fp.MediaID), "a new calibration forgets the block")
		})
	}
}

// A transient failure blocks nothing: the same title retries on the GPU.
func TestOrchestrator_TransientHWFailure_DoesNotBlockTitle(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.failOnExit = transcoding.FailureHWDeviceMemory
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger()).
		WithHWRouteBlocklist(NewHWRouteBlocklist(time.Hour, 16))
	defer o.Close()
	done := make(chan struct{}, 4)
	o.SetHWDemoteSink(func(_ context.Context, _ transcoding.FailureClass, _ string) { done <- struct{}{} })

	fp := minimalFingerprint(t)
	fp.HWAccel = "nvenc"
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	select {
	case <-done:
		t.Fatal("a transient failure must not reach the host sink")
	case <-time.After(300 * time.Millisecond):
	}
	assert.False(t, o.HWRouteBlocked(fp.MediaID))
}
