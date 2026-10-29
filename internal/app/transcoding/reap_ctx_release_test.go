package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func (p *fakeProcess) startedCtx() context.Context {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastCtx
}

// A session whose ffmpeg exits on its own must cancel its context: otherwise
// it stays registered on the producer's long-lived shutdown context forever.
func TestOrchestrator_NaturalExitReleasesSessionContext(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.failOnExit = transcoding.FailureTransient
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, testOpts(), discardLogger())
	defer o.Close()

	require.NoError(t, o.EnsureSegmentRange(context.Background(), minimalFingerprint(t), 0, 0))
	require.Eventually(t, func() bool { return o.ActiveCount() == 0 }, eventuallyWait, 5*time.Millisecond)
	ctx := proc.startedCtx()
	require.NotNil(t, ctx)
	require.Eventually(t, func() bool { return ctx.Err() != nil }, eventuallyWait, 5*time.Millisecond,
		"the session context is still live after a natural exit")
}

func TestSoftwareProducer_NaturalExitReleasesSessionContext(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.failOnExit = transcoding.FailureTransient
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/media/test.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), minimalFingerprint(t), 0, 0))
	require.Eventually(t, func() bool { return p.ActiveSessionCount() == 0 }, eventuallyWait, 5*time.Millisecond)
	ctx := proc.startedCtx()
	require.NotNil(t, ctx)
	require.Eventually(t, func() bool { return ctx.Err() != nil }, eventuallyWait, 5*time.Millisecond,
		"the session context is still live after a natural exit")
}
