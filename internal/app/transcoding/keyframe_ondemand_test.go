package transcoding_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

type fakeKfStore struct {
	mu       sync.Mutex
	saved    map[string][]int64
	rows     int
	saveErr  error
	setCalls int
}

func (f *fakeKfStore) ListVideoKeyframesMissing(context.Context, int, []string, int16) ([]string, error) {
	return nil, nil
}

func (f *fakeKfStore) MarkKeyframesUnindexable(context.Context, shared.MediaItemID, int16) (int, error) {
	return 1, nil
}

func (f *fakeKfStore) SetKeyframes(_ context.Context, id shared.MediaItemID, kf []int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls++
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	if f.saved == nil {
		f.saved = map[string][]int64{}
	}
	f.saved[string(id)] = kf
	return f.rows, nil
}

type fakeKfProber struct {
	kf    []int64
	err   error
	block bool          // when true, blocks until the ctx dies (simulates the ffprobe fallback)
	delay time.Duration // sleeps before answering (simulates the mkvgo cluster walk)
	calls atomic.Int32
	// concurrency gauge: tracks the peak number of probes running at once.
	active    atomic.Int32
	maxActive atomic.Int32
}

func (f *fakeKfProber) ProbeKeyframes(ctx context.Context, _ string) ([]int64, error) {
	f.calls.Add(1)
	cur := f.active.Add(1)
	for {
		prev := f.maxActive.Load()
		if cur <= prev || f.maxActive.CompareAndSwap(prev, cur) {
			break
		}
	}
	defer f.active.Add(-1)
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.kf, f.err
}

func TestKeyframeOnDemand_IndexesFastPath(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{kf: []int64{0, 6000, 12000}}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil)

	assert.True(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))
	assert.Equal(t, []int64{0, 6000, 12000}, store.saved["m1"])
}

func TestKeyframeOnDemand_SkipsWhilePlaybackActive(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{kf: []int64{0, 6000, 12000}}
	active := 1
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).
		WithActivePlayback(func() int { return active })

	// A viewer is watching: the on-demand probe must not run - a cue-less walk
	// on top of a live transcode is exactly what starves it.
	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"),
		"must skip while playback is active")
	assert.Equal(t, int32(0), prober.calls.Load(),
		"the prober must not be touched while playback is active")

	// Nobody watching: it indexes as usual.
	active = 0
	assert.True(t, k.TryIndex(context.Background(), "m2", "/g.mkv"))
	assert.Equal(t, int32(1), prober.calls.Load())
}

func TestKeyframeOnDemand_FailureIsNegativeCached(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{err: errors.New("no cues")}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil)

	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))
	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"),
		"second attempt within the TTL must not re-probe")
	assert.Equal(t, int32(1), prober.calls.Load(), "the failure is cached, the probe ran once")
}

func TestKeyframeOnDemand_BudgetDetachesSlowProbe(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{block: true}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).WithInlineBudget(50 * time.Millisecond)

	start := time.Now()
	ok := k.TryIndex(context.Background(), "m1", "/f.mkv")
	assert.False(t, ok)
	assert.Less(t, time.Since(start), 2*time.Second,
		"the negotiation must return at the inline budget - the mkvgo cluster walk does not honour ctx cancellation, so the wait and the work are decoupled")
}

// A probe slower than the inline budget still lands in the background: this
// playback transcodes (pinned), the NEXT one negotiates the copy rung.
func TestKeyframeOnDemand_SlowProbePersistsInBackground(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{kf: []int64{0, 6000}, delay: 150 * time.Millisecond}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).WithInlineBudget(20 * time.Millisecond)

	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"), "too slow for THIS negotiation")
	assert.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return len(store.saved["m1"]) == 2
	}, 3*time.Second, 10*time.Millisecond, "the detached probe must persist for the next playback")
}

func TestKeyframeOnDemand_WorkerWonRace(t *testing.T) {
	// rows==0 = the NULL-guarded write found the index already present (the
	// background worker beat us): still a success for the caller - the
	// keyframes exist and the decision can use them.
	store := &fakeKfStore{rows: 0}
	prober := &fakeKfProber{kf: []int64{0, 6000}}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil)

	assert.True(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))
}

func TestKeyframeOnDemand_NilAndEmptyPathSafe(t *testing.T) {
	var k *apptranscoding.KeyframeOnDemand
	assert.False(t, k.TryIndex(context.Background(), "m1", "/f.mkv"), "nil receiver is a no-op")

	k2 := apptranscoding.NewKeyframeOnDemand(&fakeKfStore{}, &fakeKfProber{}, nil)
	assert.False(t, k2.TryIndex(context.Background(), "m1", ""))
}

// A burst of first-plays on distinct freshly-added files must not spawn one
// concurrent cluster walk per file: the global slot cap bounds how many
// detached probes run at once, so the weak-NAS I/O the project targets is
// never saturated. Files that miss a slot transcode this playback and are
// left to the paced background worker.
func TestKeyframeOnDemand_GlobalConcurrencyCap(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	prober := &fakeKfProber{block: true} // every probe blocks until its hard cap
	const cap = 2
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).
		WithInlineBudget(20 * time.Millisecond).
		WithMaxConcurrent(cap)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			k.TryIndex(context.Background(), "mi-"+string(rune('a'+n)), "/f.mkv")
		}(i)
	}
	wg.Wait()
	// Give the detached goroutines that DID get a slot a moment to enter the probe.
	time.Sleep(50 * time.Millisecond)

	if got := prober.maxActive.Load(); got > cap {
		t.Fatalf("peak concurrent probes = %d, must not exceed the cap %d", got, cap)
	}
	if prober.calls.Load() == 0 {
		t.Fatal("expected at least one probe to run")
	}
}

// ============================================================================
// CFR/VFR classification on the on-demand path
// ============================================================================

type fakeOnDemandFRStore struct {
	mu       sync.Mutex
	ids      []string
	modes    []string
	versions []int16
	setErr   error
}

func (f *fakeOnDemandFRStore) SetFrameRateMode(_ context.Context, id shared.MediaItemID, mode string, v int16) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, string(id))
	f.modes = append(f.modes, mode)
	f.versions = append(f.versions, v)
	return 1, f.setErr
}

func (f *fakeOnDemandFRStore) ListVideoFrameRateStale(context.Context, int, int16, []string) ([]string, error) {
	return nil, nil
}

func (f *fakeOnDemandFRStore) ReclassifyFrameRateMode(context.Context, shared.MediaItemID, string, int16) (int, error) {
	return 0, nil
}

func (f *fakeOnDemandFRStore) targets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ids...)
}

type fakeOnDemandFRAnalyzer struct {
	mode  string
	err   error
	calls atomic.Int32
}

func (a *fakeOnDemandFRAnalyzer) AnalyzeFrameRateMode(context.Context, string) (string, error) {
	a.calls.Add(1)
	return a.mode, a.err
}

// A file indexed on demand never reappears in the backfill queue, so if the
// classification does not ride along HERE its frame-rate mode stays unknown
// forever - and the copy gate fails open on unknown. That is a VFR source
// direct-played, which is what the classifier exists to prevent.
func TestKeyframeOnDemand_ClassifiesFrameRateAlongsideTheIndex(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	frStore := &fakeOnDemandFRStore{}
	prober := &fakeKfProber{kf: []int64{0, 6000, 12000}}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).
		WithFrameRateAnalyzer(&fakeOnDemandFRAnalyzer{mode: "vfr"}, frStore)

	assert.True(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))

	assert.Equal(t, []string{"m1"}, frStore.targets(),
		"the same walk that produced the keyframes must persist the frame-rate verdict")
	assert.Equal(t, []string{"vfr"}, frStore.modes)
	assert.Equal(t, []int16{apptranscoding.FrameRateAlgoVersion},
		frStore.versions,
		"the verdict must carry the current classifier version - an unstamped row is invisible to the re-classification drain")
}

// Classification is optional and best-effort in both directions: unwired it must
// stay silent, and a failing or inconclusive analyzer must never write a verdict
// (a persisted wrong answer would be worse than no answer - it is never
// overwritten at the same version).
func TestKeyframeOnDemand_FrameRateClassificationIsBestEffort(t *testing.T) {
	cases := []struct {
		name     string
		analyzer *fakeOnDemandFRAnalyzer
		wire     bool
	}{
		{name: "unwired", wire: false},
		{name: "analyzer error", analyzer: &fakeOnDemandFRAnalyzer{err: errors.New("probe boom")}, wire: true},
		{name: "inconclusive", analyzer: &fakeOnDemandFRAnalyzer{mode: ""}, wire: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeKfStore{rows: 1}
			frStore := &fakeOnDemandFRStore{}
			prober := &fakeKfProber{kf: []int64{0, 6000}}
			k := apptranscoding.NewKeyframeOnDemand(store, prober, nil)
			if tc.wire {
				k = k.WithFrameRateAnalyzer(tc.analyzer, frStore)
			}

			assert.True(t, k.TryIndex(context.Background(), "m1", "/f.mkv"),
				"the keyframe index must succeed regardless of the classification")
			assert.Equal(t, []int64{0, 6000}, store.saved["m1"])
			assert.Empty(t, frStore.targets(), "no verdict may be persisted")
		})
	}
}

// A persist failure is logged and swallowed: the keyframes are already written
// and the negotiation must still report success, or a transient DB hiccup would
// pin an otherwise copy-eligible file to transcode.
func TestKeyframeOnDemand_FrameRatePersistFailureDoesNotFailTheIndex(t *testing.T) {
	store := &fakeKfStore{rows: 1}
	frStore := &fakeOnDemandFRStore{setErr: errors.New("db down")}
	prober := &fakeKfProber{kf: []int64{0, 6000}}
	k := apptranscoding.NewKeyframeOnDemand(store, prober, nil).
		WithFrameRateAnalyzer(&fakeOnDemandFRAnalyzer{mode: "cfr"}, frStore)

	assert.True(t, k.TryIndex(context.Background(), "m1", "/f.mkv"))
	assert.Equal(t, []int64{0, 6000}, store.saved["m1"], "the keyframes stay written")
}
