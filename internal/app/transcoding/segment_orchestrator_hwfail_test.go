package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

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
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

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
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

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
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

	select {
	case <-called:
		t.Fatal("demote sink fired on a software session")
	case <-time.After(300 * time.Millisecond):
		// expected: no demotion (usedHW=false)
	}
}

// A natural exit with a non-HW class (e.g. cancellation) must NOT demote —
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
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

	select {
	case <-called:
		t.Fatal("demote sink fired on a non-HW exit")
	case <-time.After(300 * time.Millisecond):
		// expected: no demotion
	}
}
