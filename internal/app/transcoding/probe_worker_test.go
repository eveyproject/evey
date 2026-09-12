package transcoding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

func TestProbeSignalStore(t *testing.T) {
	s := NewProbeSignalStore(0)
	for range transcoding.ProbeCeilingHitThreshold - 1 {
		s.RecordCeilingHit("a")
	}
	assert.Empty(t, s.CeilingHitCandidates(), "under the threshold is not a candidate")
	s.RecordCeilingHit("a")
	assert.Equal(t, []string{"a"}, s.CeilingHitCandidates())
	assert.Equal(t, transcoding.ProbeCeilingHitThreshold, s.Signals("a").CeilingHitSegments)
	// Capped so one long stream can't pin it high.
	for range 1000 {
		s.RecordCeilingHit("a")
	}
	assert.LessOrEqual(t, s.Signals("a").CeilingHitSegments, ceilingHitCap)
	s.Clear("a")
	assert.Empty(t, s.CeilingHitCandidates())
	assert.Zero(t, s.Signals("a").CeilingHitSegments)
	// nil-safe.
	var nilStore *ProbeSignalStore
	nilStore.RecordCeilingHit("x")
	assert.Nil(t, nilStore.CeilingHitCandidates())
}

func TestProbeSignalStore_BoundedAndConcurrent(t *testing.T) {
	s := NewProbeSignalStore(2)
	// Two hot candidates fill the store; a third new title is dropped, not a hot one.
	for _, id := range []string{"h1", "h2"} {
		for range transcoding.ProbeCeilingHitThreshold {
			s.RecordCeilingHit(id)
		}
	}
	s.RecordCeilingHit("cold")
	assert.ElementsMatch(t, []string{"h1", "h2"}, s.CeilingHitCandidates())
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { s.RecordCeilingHit("h1") })
	}
	wg.Wait()
	assert.LessOrEqual(t, s.Signals("h1").CeilingHitSegments, ceilingHitCap)
}

// --- worker fakes ---

type fakeProbeRepo struct {
	mu         sync.Mutex
	candidates []string
	entries    map[string]transcoding.ComplexityEntry
	sets       map[string]transcoding.ComplexityEntry
}

func (r *fakeProbeRepo) ListProbeCandidates(_ context.Context, _, limit int) ([]string, error) {
	if len(r.candidates) > limit {
		return r.candidates[:limit], nil
	}
	return r.candidates, nil
}
func (r *fakeProbeRepo) Get(_ context.Context, id string) (transcoding.ComplexityEntry, bool, error) {
	e, ok := r.entries[id]
	return e, ok, nil
}
func (r *fakeProbeRepo) Set(_ context.Context, id string, class transcoding.ContentClass, m transcoding.ComplexityMetrics, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sets == nil {
		r.sets = map[string]transcoding.ComplexityEntry{}
	}
	r.sets[id] = transcoding.ComplexityEntry{Class: class, Metrics: m, ComputedAt: at}
	return nil
}

type fakeItems struct{}

func (fakeItems) GetByID(_ context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	return &domainlibrary.MediaItem{ID: id, Title: "t", File: &domainlibrary.MediaFile{Path: "/f/" + string(id) + ".mkv"}}, nil
}

type fakeProber struct {
	mu      sync.Mutex
	calls   []string
	probe   transcoding.TitleProbe
	err     error
	delay   time.Duration
	started chan string
	release chan struct{}
}

func (p *fakeProber) Probe(ctx context.Context, path string, _ transcoding.TitleCost) (ProbeReportLike, error) {
	p.mu.Lock()
	p.calls = append(p.calls, path)
	p.mu.Unlock()
	if p.started != nil {
		p.started <- path
	}
	if p.release != nil {
		<-p.release
	}
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.err != nil {
		return ProbeReportLike{}, p.err
	}
	return ProbeReportLike{Probe: p.probe, Points: 2}, nil
}
func (p *fakeProber) callCount() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.calls) }

// ProbeReportLike is the shape the fake returns; the worker's TitleProber
// interface is adapted to it in the test via proberFunc.
type ProbeReportLike struct {
	Probe  transcoding.TitleProbe
	Points int
}

type proberFunc struct{ f *fakeProber }

func (a proberFunc) Probe(ctx context.Context, path string, c transcoding.TitleCost) (transcoding.TitleProbe, ProbeStats, error) {
	rep, err := a.f.Probe(ctx, path, c)
	return rep.Probe, ProbeStats{Points: rep.Points}, err
}

func canonMetrics(motion float64) transcoding.ComplexityMetrics {
	return transcoding.ComplexityMetrics{MotionMean: motion, FramesSampled: 200, MetricVersion: transcoding.CanonicalMetricVersion}
}

func newProbeWorker(repo *fakeProbeRepo, pr *fakeProber, sig ceilingHitSource) *ProbeWorker {
	return NewProbeWorker(repo, proberFunc{pr}, fakeItems{}, sig, transcoding.CostModelV2,
		ProbeWorkerOpts{BatchSize: 5, ItemPause: time.Millisecond}, nil)
}

func TestProbeWorker_ProbesOnlyCandidatesAndPersists(t *testing.T) {
	repo := &fakeProbeRepo{
		candidates: []string{"outrange", "predictable"},
		entries: map[string]transcoding.ComplexityEntry{
			"outrange":    {Class: transcoding.ClassFilmNormal, Metrics: canonMetrics(0.2)}, // below YDIFMin
			"predictable": {Class: transcoding.ClassFilmNormal, Metrics: canonMetrics(4)},   // in range: not a candidate
		},
	}
	pr := &fakeProber{probe: transcoding.TitleProbe{Protocol: transcoding.TitleProbeProtocol, ModelVersion: transcoding.CostModelV2.Version, Confirmed: true}}
	w := newProbeWorker(repo, pr, nil)
	n := w.processBatch(context.Background())
	assert.Equal(t, 1, n, "only the out-of-range title is probed")
	assert.Equal(t, []string{"/f/outrange.mkv"}, pr.calls)
	require.Contains(t, repo.sets, "outrange")
	assert.NotNil(t, repo.sets["outrange"].Metrics.Probe, "the ground truth is persisted")
	assert.NotContains(t, repo.sets, "predictable", "an in-range title is never probed nor rewritten")
}

func TestProbeWorker_CeilingHitFromSignals(t *testing.T) {
	sig := NewProbeSignalStore(0)
	for range transcoding.ProbeCeilingHitThreshold {
		sig.RecordCeilingHit("hot")
	}
	repo := &fakeProbeRepo{entries: map[string]transcoding.ComplexityEntry{
		"hot": {Class: transcoding.ClassFilmNormal, Metrics: canonMetrics(4)}, // predictable, but ceiling hit fires
	}}
	pr := &fakeProber{probe: transcoding.TitleProbe{Protocol: transcoding.TitleProbeProtocol, ModelVersion: transcoding.CostModelV2.Version}}
	w := newProbeWorker(repo, pr, sig)
	n := w.processBatch(context.Background())
	assert.Equal(t, 1, n)
	assert.Equal(t, []string{"/f/hot.mkv"}, pr.calls)
	assert.Empty(t, sig.CeilingHitCandidates(), "the signal is cleared once probed")
}

func TestProbeWorker_NoConcurrentDuplicate(t *testing.T) {
	repo := &fakeProbeRepo{candidates: []string{"x"}, entries: map[string]transcoding.ComplexityEntry{
		"x": {Class: transcoding.ClassFilmNormal, Metrics: canonMetrics(0.2)},
	}}
	pr := &fakeProber{probe: transcoding.TitleProbe{Protocol: transcoding.TitleProbeProtocol, ModelVersion: transcoding.CostModelV2.Version},
		started: make(chan string, 1), release: make(chan struct{})}
	w := newProbeWorker(repo, pr, nil)
	go w.probeOne(context.Background(), "x")
	<-pr.started // first probe is inside Probe(), holding the claim
	assert.False(t, w.probeOne(context.Background(), "x"), "a second probe of the same title is refused while one is in flight")
	close(pr.release)
	assert.Eventually(t, func() bool { return pr.callCount() == 1 }, time.Second, 5*time.Millisecond)
}

func TestProbeWorker_YieldsToPlayback(t *testing.T) {
	repo := &fakeProbeRepo{candidates: []string{"x"}, entries: map[string]transcoding.ComplexityEntry{
		"x": {Class: transcoding.ClassFilmNormal, Metrics: canonMetrics(0.2)},
	}}
	pr := &fakeProber{}
	w := newProbeWorker(repo, pr, nil).WithActivePlayback(func() int { return 1 })
	assert.Zero(t, w.processBatch(context.Background()), "a live playback skips the whole tick")
	assert.Zero(t, pr.callCount())
}

func TestProbeWorker_FailureMarkedNotRetried(t *testing.T) {
	repo := &fakeProbeRepo{candidates: []string{"bad"}, entries: map[string]transcoding.ComplexityEntry{
		"bad": {Class: transcoding.ClassFilmNormal, Metrics: canonMetrics(0.2)},
	}}
	pr := &fakeProber{err: errors.New("unreadable")}
	w := newProbeWorker(repo, pr, nil)
	w.processBatch(context.Background())
	require.Contains(t, repo.sets, "bad")
	assert.True(t, repo.sets["bad"].Metrics.Probe.Failed, "a hard failure is stamped so it is not retried at this model version")
}
