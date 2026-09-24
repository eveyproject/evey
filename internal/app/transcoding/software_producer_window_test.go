package transcoding

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyProducerWithPlan builds a copy-mode producer over a plan of n segments,
// one keyframe every 10 s.
func copyProducerWithPlan(t *testing.T, n int, opts SoftwareProducerOptions) (*SoftwareProducer, *SegmentCache, *fakeProcess, DecisionFingerprint) {
	t.Helper()
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	t.Cleanup(p.Close)
	kf := make([]int64, n)
	for i := range kf {
		kf[i] = int64(i) * 10_000
	}
	p.SetCopyMode(NewCopyPlanner(&fakeCopyStreams{streams: videoStreamWithKeyframes(kf)}, &fakeCopyItems{item: itemWithDurationMs(int64(n) * 10_000)}), nil)
	return p, cache, proc, minimalFingerprint(t)
}

// materialise writes ready segments [from, to] for the fingerprint's rung 0.
func materialise(t *testing.T, cache *SegmentCache, fp DecisionFingerprint, from, to int) {
	t.Helper()
	dir, err := cache.DirFor(fp.Hash())
	require.NoError(t, err)
	for i := from; i <= to; i++ {
		name := SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: i}.fileName(fp.UseFMP4)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), fmp4Seg(), 0o644))
	}
}

func onlySession(t *testing.T, p *SoftwareProducer, fp DecisionFingerprint) *swSession {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	list := p.sessions[fp.Hash()]
	require.Len(t, list, 1)
	return list[0]
}

// A copy session that ran CopyLeadSegs past the viewer is suspended, and a
// segment request does not wake it - only the viewer eating half the lead
// does. The idle pause keeps its own rule.
func TestCopyWindow_LeadCapPausesAndResumesOnTheViewer(t *testing.T) {
	opts := swTestOpts()
	opts.CopyLeadSegs, opts.CopyKeepBehindSegs = 10, 4
	p, cache, proc, fp := copyProducerWithPlan(t, 200, opts)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, proc.startCount())
	s := onlySession(t, p, fp)
	h := s.handle.(*fakeHandle)

	materialise(t, cache, fp, 0, 5) // 5 ahead: under the lead
	p.sweepCopyWindow()
	if pauses, _, paused := h.counts(); pauses != 0 || paused {
		t.Fatalf("under the lead the session must keep running: pauses=%d paused=%v", pauses, paused)
	}

	materialise(t, cache, fp, 6, 40) // 40 ahead of frontier 0: over the lead
	p.sweepCopyWindow()
	_, _, paused := h.counts()
	assert.True(t, paused, "far ahead of the viewer: suspended")

	// The viewer keeps pulling: a request is a cache hit that does NOT lift
	// the lead pause (the lead is still full).
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 3))
	_, resumes, paused := h.counts()
	assert.True(t, paused, "a segment request must not resume a lead-paused session")
	assert.Equal(t, 0, resumes)

	// The viewer reaches segment 36: 4 ahead, under half the lead: resumed.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 36))
	p.sweepCopyWindow()
	_, resumes, paused = h.counts()
	assert.False(t, paused, "the viewer ate the lead: resumed")
	assert.Equal(t, 1, resumes)
	assert.Equal(t, 1, proc.startCount(), "no second encoder for any of this")
}

// A starved client lifts the lead pause: bytes now, whatever the window.
func TestCopyWindow_StarvationHintLiftsTheLeadPause(t *testing.T) {
	opts := swTestOpts()
	opts.CopyLeadSegs = 10
	p, cache, _, fp := copyProducerWithPlan(t, 200, opts)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	materialise(t, cache, fp, 0, 40)
	p.sweepCopyWindow()
	h := onlySession(t, p, fp).handle.(*fakeHandle)
	if _, _, paused := h.counts(); !paused {
		t.Fatal("precondition: lead-paused")
	}
	p.NotifyClientHint(fp.MediaID, 0, 0, true, 2)
	_, _, paused := h.counts()
	assert.False(t, paused, "a starvation hint resumes even a lead-paused session")
}

// Segments more than CopyKeepBehindSegs behind the viewer are deleted, the
// session's floor rises past them, and a seek back below the floor is served
// by a fresh encoder at that keyframe instead of a wait on a file nobody will
// write again.
func TestCopyWindow_TrimsBehindTheViewerAndSpawnsOnASeekBelowTheFloor(t *testing.T) {
	opts := swTestOpts()
	opts.CopyLeadSegs, opts.CopyKeepBehindSegs = 100, 4
	p, cache, proc, fp := copyProducerWithPlan(t, 200, opts)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	materialise(t, cache, fp, 0, 30)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 20), "viewer at 20")
	p.sweepCopyWindow()

	ready, err := cache.ListReady(fp.Hash(), 0, fp.UseFMP4)
	require.NoError(t, err)
	assert.Equal(t, 16, ready[0], "everything below 20-4 is gone")
	assert.Equal(t, 30, ready[len(ready)-1], "nothing ahead is touched")
	s := onlySession(t, p, fp)
	s.mu.Lock()
	floor := s.floorSeg
	s.mu.Unlock()
	assert.Equal(t, 16, floor)

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 17), "inside the kept window: a hit")
	assert.Equal(t, 1, proc.startCount())
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 5), "below the floor: a fresh remux")
	assert.Equal(t, 2, proc.startCount())
	assert.Equal(t, 5, proc.lastOpts().StartNumber)
}

// Two viewers of the same hash: the trim follows the LOWEST one, so a slower
// tab never loses the segments it is still pulling.
func TestCopyWindow_TrimFollowsTheLowestViewer(t *testing.T) {
	opts := swTestOpts()
	opts.CopyLeadSegs, opts.CopyKeepBehindSegs = 100, 4
	opts.MaxSessionsPerHash = 2
	p, cache, proc, fp := copyProducerWithPlan(t, 200, opts)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	materialise(t, cache, fp, 0, 30)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 25), "first viewer at 25")
	// A second viewer starting at 100: no covering session in reach, a
	// second encoder.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 100))
	require.Equal(t, 2, proc.startCount())
	materialise(t, cache, fp, 100, 110)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 108))

	p.sweepCopyWindow()
	ready, err := cache.ListReady(fp.Hash(), 0, fp.UseFMP4)
	require.NoError(t, err)
	assert.Equal(t, 21, ready[0], "the cut follows the lowest viewer (25 - 4)")
	assert.Contains(t, ready, 100, "the second viewer's segments are untouched")
}

// The transcode instance never trims nor pauses for lead: its sessions are
// paced near real time and reclaimed by the idle GC.
func TestCopyWindow_TranscodeInstanceIsUntouched(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.CopyLeadSegs, opts.CopyKeepBehindSegs = 1, 1
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer p.Close()
	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	materialise(t, cache, fp, 0, 30)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 10))
	p.sweepCopyWindow()
	ready, err := cache.ListReady(fp.Hash(), 0, fp.UseFMP4)
	require.NoError(t, err)
	assert.Equal(t, 0, ready[0], "nothing trimmed")
	_, _, paused := onlySession(t, p, fp).handle.(*fakeHandle).counts()
	assert.False(t, paused, "nothing paused")
}
