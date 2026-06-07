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
)

// fakeLevelStore replays a queue of ListVideoLevelMissing results (one per call)
// and records the exclude lists it received plus every SetVideoLevels target.
type fakeLevelStore struct {
	mu        sync.Mutex
	queue     [][]string // successive ListVideoLevelMissing returns
	listErr   error
	setReturn int // rows "updated" SetVideoLevels reports
	setErr    error

	listCalls []fakeListCall
	setCalls  []string
}

type fakeListCall struct {
	limit   int
	exclude []string
}

func (s *fakeLevelStore) ListVideoLevelMissing(_ context.Context, limit int, exclude []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls = append(s.listCalls, fakeListCall{limit: limit, exclude: append([]string(nil), exclude...)})
	if s.listErr != nil {
		return nil, s.listErr
	}
	if len(s.queue) == 0 {
		return nil, nil
	}
	out := s.queue[0]
	s.queue = s.queue[1:]
	return out, nil
}

func (s *fakeLevelStore) SetVideoLevels(_ context.Context, id shared.MediaItemID, _ []*domainlibrary.MediaStream) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls = append(s.setCalls, string(id))
	return s.setReturn, s.setErr
}

func (s *fakeLevelStore) setTargets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.setCalls...)
}

func (s *fakeLevelStore) excludeOnCall(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= len(s.listCalls) {
		return nil
	}
	return s.listCalls[n].exclude
}

type fakeLevelProber struct {
	result *domainlibrary.ProbeResult
	err    error
}

func (p *fakeLevelProber) ProbeStreams(_ context.Context, _ string) (*domainlibrary.ProbeResult, error) {
	return p.result, p.err
}

func videoProbe(level int) *domainlibrary.ProbeResult {
	return &domainlibrary.ProbeResult{Streams: []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.Video, StreamIndex: 0, Codec: "h264", Level: new(level)},
		{StreamType: domainlibrary.Audio, StreamIndex: 1, Codec: "aac"},
	}}
}

func oneItemReader(id, path string) *fakeMediaReader {
	return &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		id: {ID: shared.MediaItemID(id), Title: "title-" + id, File: &domainlibrary.MediaFile{Path: path}},
	}}
}

func newLevelWorker(store LevelBackfillStore, reader MediaItemReader, prober domainlibrary.StreamProber) *LevelBackfillWorker {
	return NewLevelBackfillWorker(store, reader, prober, LevelBackfillOpts{
		BatchSize: 5, ItemPause: time.Millisecond, IdleInterval: 10 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (w *LevelBackfillWorker) attemptedHas(id string) bool {
	_, ok := w.attempted[id]
	return ok
}

func TestLevelBackfill_FillsLevelAndDoesNotMark(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{result: videoProbe(40)})

	n := w.processBatch(context.Background())
	assert.Equal(t, 1, n)
	assert.Equal(t, []string{"m1"}, store.setTargets(), "a resolved level must be persisted")
	assert.False(t, w.attemptedHas("m1"), "a successful fill must NOT be marked (it drops out of the query naturally)")
}

func TestLevelBackfill_ProbeErrorSkipsAndMarks(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{err: errors.New("ffprobe exited 1")})

	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets(), "a failed probe must never persist (no wipe risk)")
	assert.True(t, w.attemptedHas("m1"), "a failed probe must be marked so it isn't re-queried in a loop")
}

func TestLevelBackfill_EmptyProbeSkips(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"),
		&fakeLevelProber{result: &domainlibrary.ProbeResult{Streams: nil}})

	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets(), "an empty probe must never persist")
	assert.True(t, w.attemptedHas("m1"))
}

func TestLevelBackfill_NoVideoStreamSkips(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	audioOnly := &domainlibrary.ProbeResult{Streams: []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.Audio, StreamIndex: 0, Codec: "aac"},
	}}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{result: audioOnly})

	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets(), "a probe that lost the video stream must not persist (degraded → skip)")
	assert.True(t, w.attemptedHas("m1"))
}

func TestLevelBackfill_NoLevelResolvedMarks(t *testing.T) {
	// Probe succeeds and has a video stream, but the UPDATE matched nothing
	// (ffprobe reported no level): must mark so it isn't re-probed forever.
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 0}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{result: videoProbe(40)})

	w.processBatch(context.Background())
	assert.Equal(t, []string{"m1"}, store.setTargets(), "it still attempts the update")
	assert.True(t, w.attemptedHas("m1"), "no level resolved ⇒ marked to break the re-probe loop")
}

func TestLevelBackfill_MissingFileSkips(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		"m1": {ID: "m1", File: nil},
	}}
	w := newLevelWorker(store, reader, &fakeLevelProber{result: videoProbe(40)})

	w.processBatch(context.Background())
	assert.Empty(t, store.setTargets())
	assert.True(t, w.attemptedHas("m1"))
}

func TestLevelBackfill_ExcludesAttemptedOnNextQuery(t *testing.T) {
	// First batch fails m1 (marked); the second query must exclude it so the
	// worker never spins on the same stuck item.
	store := &fakeLevelStore{queue: [][]string{{"m1"}, {"m2"}}, setReturn: 1}
	reader := &fakeMediaReader{items: map[string]*domainlibrary.MediaItem{
		"m1": {ID: "m1", File: &domainlibrary.MediaFile{Path: "/media/m1.mp4"}},
		"m2": {ID: "m2", File: &domainlibrary.MediaFile{Path: "/media/m2.mp4"}},
	}}
	w := newLevelWorker(store, reader, &fakeLevelProber{err: errors.New("boom")})

	w.processBatch(context.Background()) // m1 fails → marked
	assert.Empty(t, store.excludeOnCall(0), "first query has nothing to exclude")

	w.processBatch(context.Background()) // m2 fails → marked
	assert.Equal(t, []string{"m1"}, store.excludeOnCall(1), "second query must exclude the stuck m1")
}

func TestLevelBackfill_ListErrorReturnsZero(t *testing.T) {
	store := &fakeLevelStore{listErr: errors.New("db down")}
	w := newLevelWorker(store, &fakeMediaReader{}, &fakeLevelProber{result: videoProbe(40)})
	assert.Equal(t, 0, w.processBatch(context.Background()), "a list error must idle (return 0), not crash")
}

func TestLevelBackfill_StartStopClean(t *testing.T) {
	// Store drains one batch then stays empty; the worker must idle and Stop()
	// must return promptly without hanging.
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{result: videoProbe(40)})

	w.Start(context.Background())
	require.Eventually(t, func() bool {
		return len(store.setTargets()) == 1
	}, time.Second, 2*time.Millisecond, "the worker should process the one pending item")

	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return — worker goroutine leaked")
	}
}
