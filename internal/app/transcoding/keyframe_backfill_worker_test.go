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

type fakeKeyframeStore struct {
	mu        sync.Mutex
	queue     [][]string // successive ListVideoKeyframesMissing returns
	setReturn int
	setErr    error
	listCalls []fakeListCall // reuse the level worker test's type
	setCalls  []string
}

func (s *fakeKeyframeStore) ListVideoKeyframesMissing(_ context.Context, limit int, exclude []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls = append(s.listCalls, fakeListCall{limit: limit, exclude: append([]string(nil), exclude...)})
	if len(s.queue) == 0 {
		return nil, nil
	}
	out := s.queue[0]
	s.queue = s.queue[1:]
	return out, nil
}

func (s *fakeKeyframeStore) SetKeyframes(_ context.Context, id shared.MediaItemID, _ []int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls = append(s.setCalls, string(id))
	return s.setReturn, s.setErr
}

func (s *fakeKeyframeStore) setTargets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.setCalls...)
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
