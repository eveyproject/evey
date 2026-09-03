package transcoding

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
)

type fakeKeyframeStore struct {
	mu             sync.Mutex
	queue          [][]string // successive ListVideoKeyframesMissing returns
	setReturn      int
	setErr         error
	listCalls      []fakeListCall // reuse the level worker test's type
	setCalls       []string
	frameRateCalls []string // media IDs SetFrameRateMode was called for
	frameRateModes []string // the mode arg on each call, index-aligned
	frameRateErr   error

	frameRateVersions []int16 // algoVersion arg on each SetFrameRateMode call

	staleQueue       [][]string // successive ListVideoFrameRateStale returns
	staleListCalls   []fakeStaleListCall
	staleListErr     error
	reclassifyCalls  []fakeReclassifyCall
	reclassifyReturn int
	reclassifyErr    error

	listProbeVersions []int16 // probeVersion arg on each ListVideoKeyframesMissing call
	markCalls         []fakeMarkCall
	markErr           error
}

type fakeMarkCall struct {
	id      string
	version int16
}

type fakeStaleListCall struct {
	limit   int
	version int16
	exclude []string
}

type fakeReclassifyCall struct {
	id      string
	mode    string
	version int16
}

func (s *fakeKeyframeStore) ListVideoKeyframesMissing(_ context.Context, limit int, exclude []string, probeVersion int16) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls = append(s.listCalls, fakeListCall{limit: limit, exclude: append([]string(nil), exclude...)})
	s.listProbeVersions = append(s.listProbeVersions, probeVersion)
	if len(s.queue) == 0 {
		return nil, nil
	}
	out := s.queue[0]
	s.queue = s.queue[1:]
	return out, nil
}

func (s *fakeKeyframeStore) MarkKeyframesUnindexable(_ context.Context, id shared.MediaItemID, probeVersion int16) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markCalls = append(s.markCalls, fakeMarkCall{id: string(id), version: probeVersion})
	return 1, s.markErr
}

func (s *fakeKeyframeStore) marked() []fakeMarkCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeMarkCall(nil), s.markCalls...)
}

func (s *fakeKeyframeStore) SetKeyframes(_ context.Context, id shared.MediaItemID, _ []int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls = append(s.setCalls, string(id))
	return s.setReturn, s.setErr
}

func (s *fakeKeyframeStore) SetFrameRateMode(_ context.Context, id shared.MediaItemID, mode string, v int16) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frameRateCalls = append(s.frameRateCalls, string(id))
	s.frameRateModes = append(s.frameRateModes, mode)
	s.frameRateVersions = append(s.frameRateVersions, v)
	return 1, s.frameRateErr
}

func (s *fakeKeyframeStore) ListVideoFrameRateStale(_ context.Context, limit int, v int16, exclude []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staleListCalls = append(s.staleListCalls, fakeStaleListCall{
		limit: limit, version: v, exclude: append([]string(nil), exclude...),
	})
	if len(s.staleQueue) == 0 {
		return nil, s.staleListErr
	}
	out := s.staleQueue[0]
	s.staleQueue = s.staleQueue[1:]
	return out, nil
}

func (s *fakeKeyframeStore) ReclassifyFrameRateMode(_ context.Context, id shared.MediaItemID, mode string, v int16) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reclassifyCalls = append(s.reclassifyCalls, fakeReclassifyCall{id: string(id), mode: mode, version: v})
	return s.reclassifyReturn, s.reclassifyErr
}

func (s *fakeKeyframeStore) reclassified() []fakeReclassifyCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeReclassifyCall(nil), s.reclassifyCalls...)
}

func (s *fakeKeyframeStore) staleCalls() []fakeStaleListCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeStaleListCall(nil), s.staleListCalls...)
}

func (s *fakeKeyframeStore) setTargets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.setCalls...)
}

func (s *fakeKeyframeStore) frameRateTargets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.frameRateCalls...)
}

func (s *fakeKeyframeStore) excludeOnCall(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= len(s.listCalls) {
		return nil
	}
	return s.listCalls[n].exclude
}

type fakeKeyframeProber struct {
	kf  []int64
	err error
}

func (p *fakeKeyframeProber) ProbeKeyframes(_ context.Context, _ string) ([]int64, error) {
	return p.kf, p.err
}

type fakeFrameRateAnalyzer struct {
	mode string
	err  error
}

func (a *fakeFrameRateAnalyzer) AnalyzeFrameRateMode(_ context.Context, _ string) (string, error) {
	return a.mode, a.err
}

func newKeyframeWorker(store KeyframeBackfillStore, reader MediaItemReader, prober KeyframeProber) *KeyframeBackfillWorker {
	return NewKeyframeBackfillWorker(store, reader, prober, KeyframeBackfillOpts{
		BatchSize: 5, ItemPause: time.Millisecond, IdleInterval: 10 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (w *KeyframeBackfillWorker) attemptedHas(id string) bool {
	_, ok := w.attempted[id]
	return ok
}

func TestKeyframeBackfill_IndexesAndDoesNotMark(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 3253, 6548}})
	assert.Equal(t, 1, w.processBatch(context.Background()))
	assert.Equal(t, []string{"m1"}, store.setTargets())
	assert.False(t, w.attemptedHas("m1"), "a success drops out of the query naturally, not marked")
}

func TestKeyframeBackfill_NoFrameRateAnalyzerSkipsClassification(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 3253}})
	assert.Equal(t, 1, w.processBatch(context.Background()))
	assert.Empty(t, store.frameRateTargets(), "no WithFrameRateAnalyzer wired ⇒ no frame-rate call, unchanged pre-v0.17.0 behaviour")
}

func TestKeyframeBackfill_FrameRateAnalyzerPersistsAlongsideKeyframes(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 3253}}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{mode: "vfr"}, store)
	assert.Equal(t, 1, w.processBatch(context.Background()))
	assert.Equal(t, []string{"m1"}, store.frameRateTargets(), "the SAME per-item pass persists the frame-rate mode")
	assert.Equal(t, []string{"vfr"}, store.frameRateModes)
}

func TestKeyframeBackfill_FrameRateAnalyzerErrorDoesNotFailKeyframeWrite(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 3253}}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{err: errors.New("analyze boom")}, store)
	assert.Equal(t, 1, w.processBatch(context.Background()))
	assert.Equal(t, []string{"m1"}, store.setTargets(), "the keyframe write is unaffected by a frame-rate analysis failure")
	assert.Empty(t, store.frameRateTargets(), "a failed analysis is never persisted")
	assert.False(t, w.attemptedHas("m1"), "the keyframe backfill itself still succeeded")
}

func TestKeyframeBackfill_ProbeErrorSkipsAndMarks(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{err: errors.New("ffprobe boom")})
	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets(), "a failed probe must never persist")
	assert.True(t, w.attemptedHas("m1"), "marked so it isn't re-queried in a loop")
}

func TestKeyframeBackfill_EmptyProbeMarks(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: nil})
	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets(), "no keyframes ⇒ no write")
	assert.True(t, w.attemptedHas("m1"))
	assert.Empty(t, store.marked(), "missing file ⇒ transient, never stamped unindexable")
}

func TestKeyframeBackfill_EmptyProbeOnPresentFileStampsUnindexable(t *testing.T) {
	// The prober looked at real bytes and found no extractable index (the avi
	// case): re-probing the same file at the next boot yields the same nothing.
	// Stamp the verdict so the candidate query stops re-serving it; a prober
	// that learns the container bumps KeyframeProbeAlgoVersion to retry all.
	f, err := os.CreateTemp(t.TempDir(), "*.avi")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", f.Name()), &fakeKeyframeProber{kf: nil})
	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets(), "no keyframes ⇒ no index write")
	assert.Equal(t, []fakeMarkCall{{id: "m1", version: KeyframeProbeAlgoVersion}}, store.marked(),
		"present file + empty probe ⇒ stamped unindexable at the current prober version")
}

func TestKeyframeBackfill_ProbeErrorOnPresentFileStampsUnindexable(t *testing.T) {
	// Same reasoning as the empty probe: the file is there, ffprobe/mkvgo still
	// cannot make sense of it, and would fail identically on every sweep.
	f, err := os.CreateTemp(t.TempDir(), "*.avi")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", f.Name()), &fakeKeyframeProber{err: errors.New("ffprobe boom")})
	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets())
	assert.Equal(t, []fakeMarkCall{{id: "m1", version: KeyframeProbeAlgoVersion}}, store.marked())
}

func TestKeyframeBackfill_ListCarriesProbeVersion(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 100}})
	w.processBatch(context.Background())
	require.NotEmpty(t, store.listProbeVersions)
	assert.Equal(t, KeyframeProbeAlgoVersion, store.listProbeVersions[0],
		"the candidate query must exclude rows stamped at the current prober version")
}

func TestKeyframeBackfill_NoRowUpdatedMarks(t *testing.T) {
	// Probe found keyframes but the UPDATE matched nothing (already set elsewhere):
	// mark to avoid re-probing the same heavy file forever.
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 0}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 100}})
	w.processBatch(context.Background())
	assert.Equal(t, []string{"m1"}, store.setTargets())
	assert.True(t, w.attemptedHas("m1"))
}

func TestKeyframeBackfill_ExcludesAttemptedOnNextQuery(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}, {"m2"}}, setReturn: 1}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		"m1": {ID: "m1", File: &domainlibrary.MediaFile{Path: "/media/m1.mkv"}},
		"m2": {ID: "m2", File: &domainlibrary.MediaFile{Path: "/media/m2.mkv"}},
	}}
	w := newKeyframeWorker(store, reader, &fakeKeyframeProber{err: errors.New("boom")})
	w.processBatch(context.Background())
	assert.Empty(t, store.excludeOnCall(0))
	w.processBatch(context.Background())
	assert.Equal(t, []string{"m1"}, store.excludeOnCall(1), "the stuck m1 is excluded on the next query")
}

func TestKeyframeBackfill_StartStopClean(t *testing.T) {
	store := &fakeKeyframeStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 3253}})
	w.Start(context.Background())
	require.Eventually(t, func() bool { return len(store.setTargets()) == 1 }, time.Second, 2*time.Millisecond)
	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return")
	}
}

func (s *fakeKeyframeStore) listCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.listCalls)
}

// A Nudge wakes an idle worker immediately instead of waiting the idle
// interval - the library scan-completed trigger relies on it.
func TestKeyframeBackfill_NudgeWakesIdleWorker(t *testing.T) {
	store := &fakeKeyframeStore{} // no candidates → the worker goes idle
	w := NewKeyframeBackfillWorker(store, &fakeMediaReader{}, &fakeKeyframeProber{}, KeyframeBackfillOpts{
		BatchSize: 2, ItemPause: time.Millisecond, IdleInterval: time.Hour,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.Start(context.Background())
	defer w.Stop()

	require.Eventually(t, func() bool { return store.listCallCount() >= 1 }, 2*time.Second, 5*time.Millisecond)
	before := store.listCallCount()

	w.Nudge()
	require.Eventually(t, func() bool { return store.listCallCount() > before },
		2*time.Second, 5*time.Millisecond, "a nudge must trigger a fresh candidate pass without waiting the 1h idle")
}

// NudgeAfterQuiet debounces: re-arms reset the window, and only the last one
// fires - a scan storm produces one drain, not one per event.
func TestKeyframeBackfill_NudgeAfterQuietDebounces(t *testing.T) {
	store := &fakeKeyframeStore{}
	w := NewKeyframeBackfillWorker(store, &fakeMediaReader{}, &fakeKeyframeProber{}, KeyframeBackfillOpts{
		BatchSize: 2, ItemPause: time.Millisecond, IdleInterval: time.Hour,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.Start(context.Background())
	defer w.Stop()
	require.Eventually(t, func() bool { return store.listCallCount() >= 1 }, 2*time.Second, 5*time.Millisecond)
	before := store.listCallCount()

	for range 5 {
		w.NudgeAfterQuiet(150 * time.Millisecond)
		time.Sleep(20 * time.Millisecond)
	}
	assert.Equal(t, before, store.listCallCount(), "the debounce must not fire while events keep re-arming it")
	require.Eventually(t, func() bool { return store.listCallCount() > before },
		2*time.Second, 5*time.Millisecond, "after the quiet window the nudge fires")
}

// While playback sessions are active the per-item gap switches to the polite
// pace; back to the assertive pace when the server is quiet.
func TestKeyframeBackfill_PolitePauseWhilePlaying(t *testing.T) {
	active := 0
	w := NewKeyframeBackfillWorker(&fakeKeyframeStore{}, &fakeMediaReader{}, &fakeKeyframeProber{}, KeyframeBackfillOpts{
		ItemPause: 200 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil))).WithActivePlayback(func() int { return active })

	assert.Equal(t, 200*time.Millisecond, w.pause(), "quiet server → assertive pace")
	active = 1
	assert.Equal(t, politeItemPause, w.pause(), "someone is watching → polite pace")
}

// The re-classification drain is what makes a classifier improvement reach the
// EXISTING library: without it, mkvgo v0.25.1's fix would only ever apply to
// newly scanned files, and every title the old classifier mislabelled 'vfr'
// would stay pinned to a forced transcode.
func TestFrameRateReclassify_RestampsStaleVerdicts(t *testing.T) {
	store := &fakeKeyframeStore{staleQueue: [][]string{{"m1"}}, reclassifyReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{mode: "cfr"}, store)

	assert.Equal(t, 1, w.processBatch(context.Background()))
	require.Len(t, store.reclassified(), 1)
	assert.Equal(t, fakeReclassifyCall{id: "m1", mode: "cfr", version: FrameRateAlgoVersion},
		store.reclassified()[0], "the fresh verdict is written at the CURRENT version")
	assert.Equal(t, FrameRateAlgoVersion, store.staleCalls()[0].version,
		"candidates are asked for against the current version")
}

// Priority is explicit: a pending keyframe index blocks the copy rung outright,
// a stale verdict only mis-gates it, so the drain waits its turn.
func TestFrameRateReclassify_YieldsToPendingKeyframeWork(t *testing.T) {
	store := &fakeKeyframeStore{
		queue:      [][]string{{"m1"}},
		setReturn:  1,
		staleQueue: [][]string{{"m2"}},
	}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{kf: []int64{0, 3253}}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{mode: "cfr"}, store)

	assert.Equal(t, 1, w.processBatch(context.Background()))
	assert.Empty(t, store.reclassified(), "the keyframe queue drains first")
	assert.Empty(t, store.staleCalls(), "stale candidates are not even queried while keyframes are pending")
}

// An "unknown" answer must still be PERSISTED (as a version stamp), or the row
// stays below the current version and comes back as a candidate on every
// drain - a library of non-EBML containers would be re-analyzed forever. The
// store is what protects the stored verdict from being overwritten by "";
// the worker's job is simply not to skip the write.
func TestFrameRateReclassify_UnknownStillStampsSoTheDrainConverges(t *testing.T) {
	store := &fakeKeyframeStore{staleQueue: [][]string{{"m1"}}, reclassifyReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeKeyframeProber{}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{mode: ""}, store)

	w.processBatch(context.Background())
	require.Len(t, store.reclassified(), 1, "an unknown answer is still persisted, as a stamp")
	assert.Equal(t, fakeReclassifyCall{id: "m1", mode: "", version: FrameRateAlgoVersion},
		store.reclassified()[0])
	_, skipped := w.frAttempted["m1"]
	assert.False(t, skipped, "it is not a failure: the row leaves the candidate set via its stamp")
}

// A failing analysis must not pin the queue, and must not poison the keyframe
// pass: the two skip-sets are deliberately separate.
func TestFrameRateReclassify_FailureSkipsWithoutTouchingKeyframeSkipSet(t *testing.T) {
	store := &fakeKeyframeStore{staleQueue: [][]string{{"m1"}}}
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{err: errors.New("boom")}, store)

	w.processBatch(context.Background())
	_, skipped := w.frAttempted["m1"]
	assert.True(t, skipped, "a failed analysis is skipped for the rest of the run")
	assert.False(t, w.attemptedHas("m1"), "and the keyframe pass is left free to retry the same file")
}

// The frame-rate drain must SUSPEND while anyone is watching, not merely slow
// down. One analysis walks the whole file for tens of seconds, so the per-item
// polite pause leaves the disk busy ~90% of the time - enough to starve HLS
// segment production. This regressed a live playback session in production on
// 2026-07-19 (stalled client + perpetually buffering Chromecast).
func TestFrameRateReclassify_SuspendedWhilePlaybackActive(t *testing.T) {
	store := &fakeKeyframeStore{staleQueue: [][]string{{"m1"}}, reclassifyReturn: 1}
	active := 1
	w := newKeyframeWorker(store, oneItemReader("m1", "/media/m1.mkv"), &fakeKeyframeProber{}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{mode: "cfr"}, store).
		WithActivePlayback(func() int { return active })

	assert.Equal(t, 0, w.processBatch(context.Background()), "a viewer is watching: the drain yields entirely")
	assert.Empty(t, store.staleCalls(), "candidates are not even queried while playback is active")
	assert.Empty(t, store.reclassified(), "and nothing is analyzed")

	active = 0
	assert.Equal(t, 1, w.processBatch(context.Background()), "the server is quiet again: the drain resumes")
	assert.Len(t, store.reclassified(), 1)
}

// The suspension must hold WITHIN a batch, not only at its start. A batch is
// up to batchSize whole-file walks (~20s each): with a start-only check, a
// viewer who presses play right after the batch begins competes with the
// drain for the entire remainder - the 2026-07-19 starvation all over again,
// merely shifted past one check. Bailing between items bounds the contention
// to the single analysis already in flight.
func TestFrameRateReclassify_BailsOutMidBatchWhenPlaybackStarts(t *testing.T) {
	store := &fakeKeyframeStore{staleQueue: [][]string{{"m1", "m2", "m3"}}, reclassifyReturn: 1}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		"m1": {ID: "m1", File: &domainlibrary.MediaFile{Path: "/media/m1.mkv"}},
		"m2": {ID: "m2", File: &domainlibrary.MediaFile{Path: "/media/m2.mkv"}},
		"m3": {ID: "m3", File: &domainlibrary.MediaFile{Path: "/media/m3.mkv"}},
	}}
	w := newKeyframeWorker(store, reader, &fakeKeyframeProber{}).
		WithFrameRateAnalyzer(&fakeFrameRateAnalyzer{mode: "cfr"}, store)
	// A viewer shows up right after the first analysis completes.
	w.WithActivePlayback(func() int {
		if len(store.reclassified()) >= 1 {
			return 1
		}
		return 0
	})

	handled := w.processBatch(context.Background())
	assert.Len(t, store.reclassified(), 1, "the drain must yield after the in-flight item, not finish the batch")
	assert.LessOrEqual(t, handled, 1)
}

// stoppingKeyframeProber is a probe during which an admin presses Stop: it
// reports the cancellation the real prober would see once its ffprobe dies.
type stoppingKeyframeProber struct{}

func (stoppingKeyframeProber) ProbeKeyframes(ctx context.Context, _ string) ([]int64, error) {
	bgtasks.Stop(keyframeBackfillTask)
	return nil, ctx.Err()
}

func TestKeyframeBackfill_StopMidProbeIsNotAVerdictOnTheFile(t *testing.T) {
	// The file is present and the probe "failed" - exactly the shape that
	// stamps a file unindexable. An admin stopping the worker is not that:
	// nothing may be stamped, nothing marked attempted, and the batch ends.
	f, err := os.CreateTemp(t.TempDir(), "*.mkv")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	bgtasks.RegisterControllable(keyframeBackfillTask)
	t.Cleanup(func() { bgtasks.Resume(keyframeBackfillTask) })

	store := &fakeKeyframeStore{queue: [][]string{{"m1", "m2"}}, setReturn: 1}
	w := newKeyframeWorker(store, oneItemReader("m1", f.Name()), stoppingKeyframeProber{})
	assert.Equal(t, 0, w.processBatch(context.Background()), "the batch stops at the cancelled item")
	assert.Empty(t, store.marked(), "a cancelled probe must never stamp the file unindexable")
	assert.Empty(t, store.setTargets())
	assert.False(t, w.attemptedHas("m1"), "not attempted: the next sweep must retry it")
	assert.True(t, bgtasks.Paused(keyframeBackfillTask))
}
