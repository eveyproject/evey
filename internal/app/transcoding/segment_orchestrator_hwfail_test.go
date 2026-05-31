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
