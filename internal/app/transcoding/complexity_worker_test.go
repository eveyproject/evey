package transcoding

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
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

func TestComplexityWorker_ClassifierErrorWithZeroSamplesSkipsPersist(t *testing.T) {
	const id = "media-4"
	repo := &fakeComplexityRepo{unclassified: []string{id}}
	classifier := &fakeClassifier{err: errors.New("ffmpeg exited 1")}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {
			ID:   shared.MediaItemID(id),
			File: &domainlibrary.MediaFile{Path: "/x.mkv", Duration: mustDuration(t, time.Hour)},
		},
	}}
	w := NewComplexityWorker(repo, classifier, reader, ComplexityWorkerOpts{
		BatchSize: 5,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w.processBatch(context.Background())
	assert.Empty(t, repo.setsSnapshot(), "classify failed with no samples ⇒ no row written")
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
