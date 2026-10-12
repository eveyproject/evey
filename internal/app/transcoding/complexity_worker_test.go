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

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/bgtasks"
)

// fakeComplexityRepo records Set calls and replays the configured
// unclassified queue once before going empty.
type fakeComplexityRepo struct {
	mu           sync.Mutex
	unclassified []string
	gets         map[string]domaintranscoding.ComplexityEntry
	sets         []setCall
	listErr      error
}

type setCall struct {
	mediaItemID string
	class       domaintranscoding.ContentClass
	metrics     domaintranscoding.ComplexityMetrics
}

func (r *fakeComplexityRepo) Get(_ context.Context, id string) (domaintranscoding.ComplexityEntry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gets == nil {
		return domaintranscoding.ComplexityEntry{}, false, nil
	}
	e, ok := r.gets[id]
	return e, ok, nil
}

func (r *fakeComplexityRepo) Set(_ context.Context, id string, class domaintranscoding.ContentClass, m domaintranscoding.ComplexityMetrics, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sets = append(r.sets, setCall{mediaItemID: id, class: class, metrics: m})
	return nil
}

func (r *fakeComplexityRepo) ListUnclassified(_ context.Context, _ int) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := r.unclassified
	r.unclassified = nil
	return out, nil
}

func (r *fakeComplexityRepo) setsSnapshot() []setCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]setCall, len(r.sets))
	copy(out, r.sets)
	return out
}

type fakeClassifier struct {
	out domaintranscoding.ComplexityMetrics
	err error
}

func (f *fakeClassifier) Classify(_ context.Context, _ string, _ float64, _ domaintranscoding.KnownSourceFacts) (domaintranscoding.ComplexityMetrics, error) {
	return f.out, f.err
}

func mustDuration(t *testing.T, d time.Duration) shared.Duration {
	t.Helper()
	dd, err := shared.NewDurationFromTime(d)
	require.NoError(t, err)
	return dd
}

type fakeMediaReader struct {
	items map[string]*domainlibrary.MediaItem
}

func (f *fakeMediaReader) GetByID(_ context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	if item, ok := f.items[string(id)]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}

func TestComplexityWorker_ProcessesBatchAndPersists(t *testing.T) {
	const id = "media-1"
	repo := &fakeComplexityRepo{unclassified: []string{id}}
	classifier := &fakeClassifier{out: domaintranscoding.ComplexityMetrics{
		MotionMean: 1.0, ScenecutsPerMinute: 3, SourceBitrateKbps: 2000, SourceHeight: 1080,
		FramesSampled: 30,
	}}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {
			ID:    shared.MediaItemID(id),
			Title: "Toy Story",
			File: &domainlibrary.MediaFile{
				Path:     "/media/toy.mkv",
				Duration: mustDuration(t, 90*time.Minute),
			},
		},
	}}
	w := NewComplexityWorker(repo, classifier, reader, ComplexityWorkerOpts{
		BatchSize: 5, ItemPause: 1 * time.Millisecond, IdleInterval: 1 * time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	n := w.processBatch(context.Background())
	assert.Equal(t, 1, n)
	sets := repo.setsSnapshot()
	require.Len(t, sets, 1)
	assert.Equal(t, id, sets[0].mediaItemID)
	assert.Equal(t, domaintranscoding.ClassAnimation, sets[0].class,
		"low YDEV + low scenecuts + low bitrate ⇒ animation")
}

func TestComplexityWorker_SkipsItemWithoutFile(t *testing.T) {
	const id = "media-2"
	repo := &fakeComplexityRepo{unclassified: []string{id}}
	classifier := &fakeClassifier{}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {ID: shared.MediaItemID(id), File: nil},
	}}
	w := NewComplexityWorker(repo, classifier, reader, ComplexityWorkerOpts{
		BatchSize: 5,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w.processBatch(context.Background())
	assert.Empty(t, repo.setsSnapshot(), "items without a file must not be classified")
}

func TestComplexityWorker_SkipsItemWithZeroDuration(t *testing.T) {
	const id = "media-3"
	repo := &fakeComplexityRepo{unclassified: []string{id}}
	classifier := &fakeClassifier{}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {
			ID:   shared.MediaItemID(id),
			File: &domainlibrary.MediaFile{Path: "/x.mkv", Duration: shared.Duration{}},
		},
	}}
	w := NewComplexityWorker(repo, classifier, reader, ComplexityWorkerOpts{
		BatchSize: 5,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w.processBatch(context.Background())
	assert.Empty(t, repo.setsSnapshot(), "zero duration ⇒ classifier can't sample, skip")
}

func TestComplexityWorker_ClassifierErrorOnMissingFileSkipsPersist(t *testing.T) {
	// The file is gone (unmounted share, deleted) - a transient condition the
	// next sweep may find healed. Persisting a verdict now would freeze a
	// classification the real bytes never got a chance to inform.
	const id = "media-4"
	repo := &fakeComplexityRepo{unclassified: []string{id}}
	classifier := &fakeClassifier{err: errors.New("ffmpeg exited 1")}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {
			ID:   shared.MediaItemID(id),
			File: &domainlibrary.MediaFile{Path: "/nonexistent/x.mkv", Duration: mustDuration(t, time.Hour)},
		},
	}}
	w := NewComplexityWorker(repo, classifier, reader, ComplexityWorkerOpts{
		BatchSize: 5,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w.processBatch(context.Background())
	assert.Empty(t, repo.setsSnapshot(), "missing file ⇒ transient, no row written, retry later")
}

func TestComplexityWorker_ClassifierErrorOnPresentFilePersistsFallback(t *testing.T) {
	// The file IS there and ffmpeg still hard-failed: retrying next sweep would
	// fail identically forever (the 2026-07-26 prod loop: the same unclassifiable
	// items re-listed every batch, 24 attempts/min around the clock). Settle the
	// question with the sample-free heuristic instead of never persisting.
	const id = "media-5"
	f, err := os.CreateTemp(t.TempDir(), "*.mkv")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	repo := &fakeComplexityRepo{unclassified: []string{id}}
	classifier := &fakeClassifier{err: errors.New("ffmpeg exited 1")}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {
			ID:   shared.MediaItemID(id),
			File: &domainlibrary.MediaFile{Path: f.Name(), Duration: mustDuration(t, time.Hour)},
		},
	}}
	w := NewComplexityWorker(repo, classifier, reader, ComplexityWorkerOpts{
		BatchSize: 5,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w.processBatch(context.Background())
	sets := repo.setsSnapshot()
	require.Len(t, sets, 1, "permanent classify failure must persist a fallback verdict")
	assert.Equal(t, id, sets[0].mediaItemID)
	assert.Equal(t, domaintranscoding.ClassFilmNormal, sets[0].class,
		"no samples + no known facts ⇒ the sample-free default")
	assert.Zero(t, sets[0].metrics.FramesSampled)
}

func TestComplexityWorker_StartStopIdempotent(t *testing.T) {
	// A double Start must not spawn a second loop (double-close on done),
	// and a double Stop must not re-close the stop channel. Either would
	// panic; this asserts the sync.Once guards hold.
	repo := &fakeComplexityRepo{}
	w := NewComplexityWorker(repo, &fakeClassifier{}, &fakeMediaReader{},
		ComplexityWorkerOpts{IdleInterval: time.Hour},
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	require.NotPanics(t, func() {
		w.Start(context.Background())
		w.Start(context.Background()) // no-op
		w.Stop()
		w.Stop() // no-op
	})
}

func TestComplexityWorker_StopBetweenItems(t *testing.T) {
	// Long queue, slow pause; stop after the first item, second should
	// never run.
	repo := &fakeComplexityRepo{unclassified: []string{"a", "b", "c"}}
	classifier := &fakeClassifier{out: domaintranscoding.ComplexityMetrics{FramesSampled: 1}}
	items := map[string]*domainlibrary.MediaItem{}
	for _, id := range []string{"a", "b", "c"} {
		items[id] = &domainlibrary.MediaItem{
			ID:   shared.MediaItemID(id),
			File: &domainlibrary.MediaFile{Path: "/x.mkv", Duration: mustDuration(t, time.Hour)},
		}
	}
	reader := &fakeMediaReader{items: items}
	w := NewComplexityWorker(repo, classifier, reader, ComplexityWorkerOpts{
		BatchSize: 5, ItemPause: 200 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- w.processBatch(ctx) }()
	// Let the first item complete then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	n := <-done
	assert.LessOrEqual(t, n, 2, "cancellation must stop the loop before all three items")
}

// The doc comment on startTranscodingWorkers already promised that EVERY
// transcoding worker yields to live playback - this one did not, so a viewer
// competed with nice -19 ffmpeg bursts the shaper only SIGSTOPs a beat later.
func TestComplexityWorker_SkipsTheBatchWhilePlaybackIsActive(t *testing.T) {
	repo := &fakeComplexityRepo{unclassified: []string{"a", "b", "c"}}
	classifier := &fakeClassifier{out: domaintranscoding.ComplexityMetrics{FramesSampled: 1}}
	w := NewComplexityWorker(repo, classifier, &fakeMediaReader{},
		ComplexityWorkerOpts{BatchSize: 5, IdleInterval: time.Hour},
		slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithActivePlayback(func() int { return 1 })

	require.True(t, w.playbackActive(), "one active stream must gate the worker")

	// The run loop must not reach processBatch at all: assert through the repo,
	// which records every classification attempt.
	w.Start(context.Background())
	time.Sleep(50 * time.Millisecond)
	w.Stop()

	assert.Empty(t, repo.setsSnapshot(), "no item may be classified while a stream is being served")
}

func TestComplexityWorker_RunsWhenNothingIsPlaying(t *testing.T) {
	w := NewComplexityWorker(&fakeComplexityRepo{}, &fakeClassifier{}, &fakeMediaReader{},
		ComplexityWorkerOpts{IdleInterval: time.Hour},
		slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithActivePlayback(func() int { return 0 })
	assert.False(t, w.playbackActive())
}

// Background work must not stall forever because a dependency was not wired -
// an unwired counter means "cannot tell", not "assume someone is watching".
func TestComplexityWorker_UnwiredCounterDoesNotGate(t *testing.T) {
	w := NewComplexityWorker(&fakeComplexityRepo{}, &fakeClassifier{}, &fakeMediaReader{},
		ComplexityWorkerOpts{IdleInterval: time.Hour},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.False(t, w.playbackActive())
}

// stoppingClassifier is a classification during which an admin presses Stop.
type stoppingClassifier struct{}

func (stoppingClassifier) Classify(ctx context.Context, _ string, _ float64, _ domaintranscoding.KnownSourceFacts) (domaintranscoding.ComplexityMetrics, error) {
	bgtasks.Stop(complexityTask)
	return domaintranscoding.ComplexityMetrics{}, ctx.Err()
}

func TestComplexityWorker_StopMidClassifyPersistsNoFallback(t *testing.T) {
	// Present file + hard failure is the shape that settles the item with the
	// sample-free fallback class. A cancellation is not a failure of the
	// file: no class may be written for it.
	const id = "media-stop"
	f, err := os.CreateTemp(t.TempDir(), "*.mkv")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	bgtasks.RegisterControllable(complexityTask)
	t.Cleanup(func() { bgtasks.Resume(complexityTask) })

	repo := &fakeComplexityRepo{unclassified: []string{id}}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {
			ID:   shared.MediaItemID(id),
			File: &domainlibrary.MediaFile{Path: f.Name(), Duration: mustDuration(t, time.Hour)},
		},
	}}
	w := NewComplexityWorker(repo, stoppingClassifier{}, reader, ComplexityWorkerOpts{
		BatchSize: 5,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w.processBatch(context.Background())
	assert.Empty(t, repo.setsSnapshot(), "a cancelled classification must persist nothing")
	assert.True(t, bgtasks.Paused(complexityTask))
}
