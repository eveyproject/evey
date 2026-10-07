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

	stampCalls []fakeStampCall

	idetReturn int
	idetErr    error
	idetCalls  []fakeIdetCall
}

type fakeListCall struct {
	limit   int
	exclude []string
}

type fakeIdetCall struct {
	id         string
	interlaced bool
}

// fakeInterlaceProber is a fakeLevelProber that also satisfies InterlaceDetector.
type fakeInterlaceProber struct {
	*fakeLevelProber
	interlaced bool
	idetErr    error
}

func (p *fakeInterlaceProber) ProbeInterlaceDetect(_ context.Context, _ string) (bool, error) {
	return p.interlaced, p.idetErr
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

func (s *fakeLevelStore) MarkVideoLevelProbed(_ context.Context, id shared.MediaItemID, version int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stampCalls = append(s.stampCalls, fakeStampCall{id: string(id), version: version})
	return 1, nil
}

func (s *fakeLevelStore) stamps() []fakeStampCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeStampCall(nil), s.stampCalls...)
}

type fakeStampCall struct {
	id      string
	version int
}

func (s *fakeLevelStore) SetIdetTelecine(_ context.Context, _ shared.MediaItemID, _ bool) (int, error) {
	return 1, nil
}

func (s *fakeLevelStore) SetIdetInterlaced(_ context.Context, id shared.MediaItemID, interlaced bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idetCalls = append(s.idetCalls, fakeIdetCall{id: string(id), interlaced: interlaced})
	return s.idetReturn, s.idetErr
}

func (s *fakeLevelStore) idetTargets() []fakeIdetCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeIdetCall(nil), s.idetCalls...)
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

// This worker had NO active-playback guard at all (found 2026-07-19 while
// auditing the starvation-incident fix): it kept forking header probes every
// few seconds while someone was watching. A header probe is far lighter than
// a full-file walk, but on the weak-NAS target every competing disk read
// counts - and the backfill is never urgent, a viewer always is.
func TestLevelBackfill_SuspendedWhilePlaybackActive(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	active := 1
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{result: videoProbe(40)}).
		WithActivePlayback(func() int { return active })

	assert.Equal(t, 0, w.processBatch(context.Background()), "a viewer is watching: the backfill yields entirely")
	assert.Empty(t, store.listCalls, "candidates are not even queried while playback is active")

	active = 0
	assert.Equal(t, 1, w.processBatch(context.Background()), "the server is quiet again: the backfill resumes")
	assert.Equal(t, []string{"m1"}, store.setTargets())
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

// A stream the prober never fills must be settled by the probe that found
// nothing, or it is re-probed at every boot (stuck "40/41").
func TestLevelBackfill_SuccessfulProbeWithNothingResolvedStampsVersion(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 0}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{result: videoProbe(40)})

	w.processBatch(context.Background())
	assert.Equal(t, []fakeStampCall{{id: "m1", version: domainlibrary.LevelProbeVersion}}, store.stamps())
}

func TestLevelBackfill_FailedProbeDoesNotStampVersion(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), &fakeLevelProber{err: errors.New("ffprobe exited 1")})

	w.processBatch(context.Background())
	assert.Empty(t, store.stamps(), "a failed probe settles nothing: the stream stays a candidate")
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
		t.Fatal("Stop() did not return - worker goroutine leaked")
	}
}

// --- idet fallback (B3.1) ---

func legacyProbe(codec string, fieldOrder *string) *domainlibrary.ProbeResult {
	return &domainlibrary.ProbeResult{Streams: []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.Video, StreamIndex: 0, Codec: codec, FieldOrder: fieldOrder},
	}}
}

// A legacy mpeg2 source with no field-order flag, idet says interlaced → the
// verdict (true) is persisted so the builder switches to deint=all.
func TestLevelBackfill_IdetDetectsUnflaggedInterlaced(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 0, idetReturn: 1}
	prober := &fakeInterlaceProber{
		fakeLevelProber: &fakeLevelProber{result: legacyProbe("mpeg2video", nil)},
		interlaced:      true,
	}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mpg"), prober)

	ok := w.backfillOne(context.Background(), "m1")
	assert.True(t, ok, "an idet verdict counts as progress (row updated)")
	got := store.idetTargets()
	require.Len(t, got, 1)
	assert.Equal(t, fakeIdetCall{id: "m1", interlaced: true}, got[0])
}

// idet says progressive → FALSE must still be persisted (else the NULL-keyed
// candidate query re-runs the decode forever).
func TestLevelBackfill_IdetPersistsProgressiveVerdict(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 0, idetReturn: 1}
	prober := &fakeInterlaceProber{
		fakeLevelProber: &fakeLevelProber{result: legacyProbe("mpeg4", nil)},
		interlaced:      false,
	}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.avi"), prober)

	w.backfillOne(context.Background(), "m1")
	got := store.idetTargets()
	require.Len(t, got, 1)
	assert.False(t, got[0].interlaced, "a progressive verdict must persist as FALSE")
}

// A flag-tagged interlaced source skips the decode: deint=interlaced already
// handles it, so the verdict short-circuits to FALSE (no idet probe).
func TestLevelBackfill_IdetSkipsFlaggedInterlaced(t *testing.T) {
	// Both prober vocabularies: ffprobe's field order and mkvgo's verdict.
	for _, flag := range []string{"tt", "interlaced"} {
		store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 0, idetReturn: 1}
		prober := &fakeInterlaceProber{
			fakeLevelProber: &fakeLevelProber{result: legacyProbe("mpeg2video", &flag)},
			interlaced:      true, // would be returned IF the probe ran
			idetErr:         nil,
		}
		w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mpg"), prober)

		w.backfillOne(context.Background(), "m1")
		got := store.idetTargets()
		require.Len(t, got, 1, flag)
		assert.False(t, got[0].interlaced, "%s: a prober-flagged source persists FALSE without an idet decode", flag)
	}
}

// A modern codec is never an idet candidate (no decode, no idet write).
func TestLevelBackfill_IdetSkipsModernCodec(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 1}
	prober := &fakeInterlaceProber{
		fakeLevelProber: &fakeLevelProber{result: videoProbe(40)}, // h264
		interlaced:      true,
	}
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mp4"), prober)

	w.backfillOne(context.Background(), "m1")
	assert.Empty(t, store.idetTargets(), "h264 must not trigger an idet probe")
}

// A prober without the InterlaceDetector capability simply skips the phase.
func TestLevelBackfill_IdetSkippedWhenProberLacksCapability(t *testing.T) {
	store := &fakeLevelStore{queue: [][]string{{"m1"}}, setReturn: 0}
	// plain fakeLevelProber does NOT implement InterlaceDetector
	w := newLevelWorker(store, oneItemReader("m1", "/media/m1.mpg"),
		&fakeLevelProber{result: legacyProbe("mpeg2video", nil)})

	w.backfillOne(context.Background(), "m1")
	assert.Empty(t, store.idetTargets())
}
