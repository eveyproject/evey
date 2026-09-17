package maintenance_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// countingStreams reports a playback count the test can flip at will.
type countingStreams struct {
	active atomic.Int32
	polls  atomic.Int32
}

func (c *countingStreams) ActivePlaybackCount() int {
	c.polls.Add(1)
	return int(c.active.Load())
}

// manyFiles writes n real .mkv files into a temp dir: the scan phase stats
// every candidate and drops the ones it cannot read, so pure in-memory paths
// would leave the batch with nothing to process.
func manyFiles(t *testing.T, n int) []appmaintenance.MediaRef {
	t.Helper()
	dir := t.TempDir()
	out := make([]appmaintenance.MediaRef, 0, n)
	for i := range n {
		name := fmt.Sprintf("f%03d.mkv", i)
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("not-really-matroska"), 0o644))
		out = append(out, appmaintenance.MediaRef{MediaID: fmt.Sprintf("m%03d", i), Path: p})
	}
	return out
}

// "Repair all" on a big library used to be unstoppable: the batch ran on
// context.Background(), the file loop never looked at cancellation, and no API
// or signal could reach it. An operator who started a multi-hour rewrite of the
// library disk by mistake had to wait it out.
func TestSeekIndex_CancelStopsBatchBetweenFiles(t *testing.T) {
	files := manyFiles(t, 40)
	gate := make(chan struct{})
	reidx := &fakeReindexer{hasIndex: map[string]bool{}, gate: gate}
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: files}, nil)

	require.NoError(t, svc.StartReindexLibrary(t.Context(), "lib1", appmaintenance.ReindexRunOptions{}))

	// Let the batch reach the first file and block inside the reindexer, so we
	// cancel a job that is genuinely mid-flight rather than one still scanning.
	require.Eventually(t, func() bool {
		return reidx.reindexCount() >= 1 || svc.Status().Phase == appmaintenance.PhaseProcessing
	}, 5*time.Second, 10*time.Millisecond, "batch never reached the processing phase")

	assert.True(t, svc.Cancel(), "Cancel must report that it stopped a running job")
	close(gate)

	waitIdle(t, svc)
	assert.Less(t, reidx.reindexCount(), len(files),
		"a cancelled batch must stop early, but it processed every one of the %d files", len(files))
}

func TestSeekIndex_CancelReportsFalseWhenNothingRuns(t *testing.T) {
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{}}, fakeCatalog{}, nil)
	assert.False(t, svc.Cancel(), "Cancel must report false when no job is running")
}

// The 2026-07-19 incident: a background rewrite of the library disk starved
// live playback of I/O. Every other heavy worker consults the playback count
// before competing for the disk; this one never did.
func TestSeekIndex_BatchYieldsWhilePlaybackIsActive(t *testing.T) {
	files := manyFiles(t, 5)
	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	streams := &countingStreams{}
	streams.active.Store(1) // someone is watching

	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: files}, nil).
		WithActiveStreamChecker(streams).
		WithIdlePollInterval(10 * time.Millisecond)

	require.NoError(t, svc.StartReindexLibrary(t.Context(), "lib1", appmaintenance.ReindexRunOptions{}))

	// Give it room to misbehave: if the batch ignored playback it would burn
	// through all 5 files well inside this window.
	require.Eventually(t, func() bool { return streams.polls.Load() > 2 }, 5*time.Second, 10*time.Millisecond,
		"the batch never consulted the playback count")
	assert.Zero(t, reidx.reindexCount(), "no file may be rewritten while playback is active")

	streams.active.Store(0) // last viewer stops
	waitIdle(t, svc)
	assert.Equal(t, len(files), reidx.reindexCount(), "the batch must resume once playback stops")
}

// reindexCount reads the fake's tally under its own lock. The batch runs on
// another goroutine, so len(reidx.reindexed) would be a data race.
func (f *fakeReindexer) reindexCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reindexed)
}

// A stopped job says so. Done < Total with no error and no failure used to
// read exactly like a batch that ran out of work; the operator could not tell
// "finished" from "stopped".
func TestSeekIndex_CancelledJobIsReportedAsCancelled(t *testing.T) {
	files := manyFiles(t, 20)
	gate := make(chan struct{})
	reidx := &fakeReindexer{hasIndex: map[string]bool{}, gate: gate}
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: files}, nil)

	require.NoError(t, svc.StartReindexLibrary(t.Context(), "lib1", appmaintenance.ReindexRunOptions{}))
	require.Eventually(t, func() bool { return svc.Status().Phase == appmaintenance.PhaseProcessing }, 5*time.Second, 10*time.Millisecond)
	require.True(t, svc.Cancel())
	close(gate)
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.True(t, st.Last.Cancelled)
	assert.Empty(t, st.Last.Err, "a cancellation is not an error")

	// A job that runs to its end is not.
	require.NoError(t, svc.StartReindexLibrary(t.Context(), "lib1", appmaintenance.ReindexRunOptions{}))
	st = waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.False(t, st.Last.Cancelled)
}

// Shutdown: the server's lifetime context is the job's parent, so the signal
// itself stops the batch at its next file, and Stop waits for the summary to
// land before returning - the journal and the marker are written under a live
// pool, not raced against db.Close.
func TestSeekIndex_StopOnServerShutdownFinishesTheJob(t *testing.T) {
	files := manyFiles(t, 40)
	gate := make(chan struct{})
	reidx := &fakeReindexer{hasIndex: map[string]bool{}, gate: gate}
	serverCtx, shutdown := context.WithCancel(context.Background())
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: files}, nil).WithBaseContext(serverCtx)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "lib1", appmaintenance.ReindexRunOptions{}))
	require.Eventually(t, func() bool { return svc.Status().Phase == appmaintenance.PhaseProcessing }, 5*time.Second, 10*time.Millisecond)

	shutdown() // SIGTERM
	close(gate)
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return once the job finished")
	}
	st := svc.Status()
	assert.False(t, st.Running, "Stop returns only once the job has finished")
	require.NotNil(t, st.Last)
	assert.True(t, st.Last.Cancelled, "a job stopped by the server going down is reported as cancelled")
	assert.Less(t, reidx.reindexCount(), len(files))
}

// Stop with nothing running is a no-op that returns at once.
func TestSeekIndex_StopIsANoopWhenIdle(t *testing.T) {
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{}}, fakeCatalog{}, nil)
	start := time.Now()
	svc.Stop()
	assert.Less(t, time.Since(start), time.Second)
}

// A cancellation that lands while a file is open is flow control, not a
// verdict: the file in flight must not come back "unreadable: context
// canceled" with a failure in a summary that also says cancelled.
func TestSeekIndex_CancelDuringAProbeIsNotAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	slow := filepath.Join(dir, "slow.mkv")
	require.NoError(t, os.WriteFile(slow, []byte("x"), 0o644))
	gate := newScanGate(slow)
	gate.armed.Store(true)
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{slow: {Healthy: true}}, gate: gate}
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{slow: true}},
		fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: slow}}}, nil).WithDiagnoser(diag)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	<-gate.started
	require.True(t, svc.Cancel())
	close(gate.release)
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.True(t, st.Last.Cancelled)
	assert.Zero(t, st.Last.Failed, "the interrupted probe is not a failure: %+v", st.Last.Failures)
	_, _, cached := svc.CachedScan("")
	assert.False(t, cached, "a cancelled scan leaves no half result behind")
}
