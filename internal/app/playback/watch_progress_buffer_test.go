package playback_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/domain/playback"
	pbfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustDurationMs is a test helper that panics on a negative input - tests
// that pass real (non-negative) literals stay terse without sprinkling
// error-handling boilerplate at every call site.
func mustDurationMs(ms int64) shared.Duration {
	d, err := shared.NewDurationMs(ms)
	if err != nil {
		panic(err)
	}
	return d
}

// countingProgressRepo records every Upsert so tests can verify whether the
// hot path hit the DB. Embeds the fake to keep the rest of the surface intact.
type countingProgressRepo struct {
	*pbfakes.FakeWatchProgressRepository
	calls atomic.Int64
}

func newCountingProgressRepo() *countingProgressRepo {
	return &countingProgressRepo{FakeWatchProgressRepository: pbfakes.NewFakeWatchProgressRepository()}
}

func (r *countingProgressRepo) Upsert(ctx context.Context, wp *playback.WatchProgress) error {
	r.calls.Add(1)
	return r.FakeWatchProgressRepository.Upsert(ctx, wp)
}

// UpsertMany must be overridden here too: Go embedding has no virtual
// dispatch, so the embedded fake's UpsertMany would call ITS OWN Upsert and
// the counter would read zero writes for a flush that persisted everything.
// Counts only on SUCCESS: a failed batch is followed by the per-entry salvage
// loop, whose Upsert calls count their own attempts - counting both would
// double-book the same entries.
func (r *countingProgressRepo) UpsertMany(ctx context.Context, entries []*playback.WatchProgress) error {
	if err := r.FakeWatchProgressRepository.UpsertMany(ctx, entries); err != nil {
		return err
	}
	r.calls.Add(int64(len(entries)))
	return nil
}

func (r *countingProgressRepo) Calls() int64 { return r.calls.Load() }

func TestWatchProgressBuffer_EnqueueDoesNotHitRepo(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, 10*time.Millisecond)

	for i := range 10 {
		_ = i
		buf.Enqueue(&playback.WatchProgress{
			ProfileID:    "p1",
			MediaItemID:  "m1",
			Position:     mustDurationMs(int64(i * 1000)),
			LastPlayedAt: time.Now(),
		})
	}

	// 10 Enqueues, 0 Upserts - the hot path stays off the DB.
	assert.Equal(t, int64(0), repo.Calls())
	assert.Equal(t, 1, buf.Pending(), "same key should fold to a single entry")
}

func TestWatchProgressBuffer_StopWithoutStartDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)

	// Stop() before any Start(): doneCh is never closed (no flush goroutine), so
	// the old `<-doneCh` blocked forever. It must now return promptly.
	done := make(chan struct{})
	go func() {
		buf.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() deadlocked when called without a prior Start()")
	}
}

func TestWatchProgressBuffer_FlushPersistsLatestPerKey(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour) // no auto-tick

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(1000), LastPlayedAt: time.Now()})
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(5000), LastPlayedAt: time.Now()})
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p2", MediaItemID: "m2", Position: mustDurationMs(2000), LastPlayedAt: time.Now()})

	n := buf.Flush(context.Background())
	assert.Equal(t, 2, n)
	assert.Equal(t, 0, buf.Pending())

	wp1, err := repo.GetByProfileAndMedia(context.Background(), "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(5000), wp1.Position.Milliseconds(), "latest position wins")

	wp2, err := repo.GetByProfileAndMedia(context.Background(), "p2", "m2")
	require.NoError(t, err)
	assert.Equal(t, int64(2000), wp2.Position.Milliseconds())
}

func TestWatchProgressBuffer_PeriodicFlush(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buf.Start(ctx)
	defer buf.Stop()

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(1000), LastPlayedAt: time.Now()})

	require.Eventually(t, func() bool {
		return repo.Calls() >= 1 && buf.Pending() == 0
	}, 2*time.Second, 2*time.Millisecond)
}

func TestWatchProgressBuffer_StopFlushesPending(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	// Long interval so the test only succeeds if Stop drains, not the ticker.
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buf.Start(ctx)

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(9000), LastPlayedAt: time.Now()})
	require.Equal(t, int64(0), repo.Calls(), "ticker should not have fired yet")

	buf.Stop()

	assert.Equal(t, int64(1), repo.Calls(), "Stop must drain pending writes")
	wp, err := repo.GetByProfileAndMedia(context.Background(), "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(9000), wp.Position.Milliseconds())
}

func TestWatchProgressBuffer_CtxCancelTriggersFinalFlush(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	buf.Start(ctx)

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(4242), LastPlayedAt: time.Now()})
	cancel()

	// Stop blocks on the goroutine's defer-flush, so by the time it returns
	// the persist has happened (the deferred Flush uses a fresh context).
	buf.Stop()
	assert.Equal(t, int64(1), repo.Calls())
}

func TestWatchProgressBuffer_FlushSkipsErrorsButContinues(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	repo.Err = errors.New("boom")
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", LastPlayedAt: time.Now()})
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p2", MediaItemID: "m2", LastPlayedAt: time.Now()})

	n := buf.Flush(context.Background())
	assert.Equal(t, 0, n, "all writes errored")
	assert.Equal(t, int64(2), repo.Calls(), "every entry is still attempted")
	assert.Equal(t, 0, buf.Pending(), "snapshot drains regardless of repo errors")
}

func TestWatchProgressBuffer_NilEnqueueIsNoop(t *testing.T) {
	t.Parallel()
	buf := appplayback.NewWatchProgressBuffer(newCountingProgressRepo(), time.Hour)
	buf.Enqueue(nil)
	assert.Equal(t, 0, buf.Pending())
}

func TestWatchProgressBuffer_ZeroIntervalUsesDefault(t *testing.T) {
	t.Parallel()
	// Smoke test: zero interval must not divide-by-zero or busy-loop.
	buf := appplayback.NewWatchProgressBuffer(newCountingProgressRepo(), 0)
	ctx, cancel := context.WithCancel(context.Background())
	buf.Start(ctx)
	cancel()
	buf.Stop()
}

func TestWatchProgressBuffer_ConcurrentEnqueueAndFlush(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, 2*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buf.Start(ctx)
	defer buf.Stop()

	const writers = 8
	const perWriter = 50
	var wg sync.WaitGroup
	wg.Add(writers)
	for w := range writers {
		go func(w int) {
			defer wg.Done()
			// Same key across all writers stresses the fold + race detector
			// against the periodic flusher draining concurrently.
			for i := range perWriter {
				buf.Enqueue(&playback.WatchProgress{
					ProfileID:    "p1",
					MediaItemID:  "m1",
					Position:     mustDurationMs(int64(w*1000 + i)),
					LastPlayedAt: time.Now(),
				})
			}
		}(w)
	}
	wg.Wait()

	// Eventually the ticker drains everything for the single key.
	require.Eventually(t, func() bool {
		return buf.Pending() == 0 && repo.Calls() >= 1
	}, 2*time.Second, 5*time.Millisecond)
}

// TestWatchProgressBuffer_Enqueue_IgnoresStaleTick proves that a late-arriving
// tick with an OLDER LastPlayedAt (e.g. a second device whose write was in
// flight, or a network reorder) cannot overwrite an already-buffered fresher
// entry before it is even flushed to the repo. Without this, the buffer's
// fold-by-key would silently drop the fresher position in memory - the DB's
// own freshness guard never gets a chance to reject it because only one
// entry per key ever reaches Upsert.
func TestWatchProgressBuffer_Enqueue_IgnoresStaleTick(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)

	now := time.Now()
	older := now.Add(-10 * time.Second)

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(90000), LastPlayedAt: now})
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(20000), LastPlayedAt: older})

	n := buf.Flush(context.Background())
	assert.Equal(t, 1, n)

	wp, err := repo.GetByProfileAndMedia(context.Background(), "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(90000), wp.Position.Milliseconds(), "stale in-flight tick must not overwrite the fresher buffered one")
}

// TestWatchProgressBuffer_Enqueue_AppliesVoluntaryRewind mirrors the repo-level
// test: a fresher tick reporting a lower position (the user rewound) must
// still replace the buffered entry.
func TestWatchProgressBuffer_Enqueue_AppliesVoluntaryRewind(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)

	now := time.Now()
	later := now.Add(10 * time.Second)

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(90000), LastPlayedAt: now})
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", Position: mustDurationMs(20000), LastPlayedAt: later})

	n := buf.Flush(context.Background())
	assert.Equal(t, 1, n)

	wp, err := repo.GetByProfileAndMedia(context.Background(), "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(20000), wp.Position.Milliseconds(), "a fresher tick reporting a rewind must replace the buffered one")
}

// --- Integration: CommandService with buffer wired in ---

func TestCommandService_ReportProgressBySession_BuffersWrite(t *testing.T) {
	t.Parallel()
	pr := newCountingProgressRepo()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	buf := appplayback.NewWatchProgressBuffer(pr, time.Hour)
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus, appplayback.WithWatchProgressBuffer(buf))

	ctx := context.Background()
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// Three player ticks within one flush window: zero DB upserts.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 5000, false, ""))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 10000, false, ""))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 15000, false, ""))

	assert.Equal(t, int64(0), pr.Calls(), "buffer should swallow upserts")

	// Manual flush persists the latest folded state.
	persisted := buf.Flush(ctx)
	assert.Equal(t, 1, persisted)
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(15000), wp.Position.Milliseconds(), "latest position wins after fold")
}

func TestCommandService_ReportProgressBySession_NoBufferStaysSynchronous(t *testing.T) {
	t.Parallel()
	pr := newCountingProgressRepo()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus) // no buffer

	ctx := context.Background()
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 5000, false, ""))
	assert.Equal(t, int64(1), pr.Calls(), "without buffer, every report hits DB")
}

func TestCommandService_StartStopProgressFlusher_NoOpWithoutBuffer(t *testing.T) {
	t.Parallel()
	pr := newCountingProgressRepo()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus)

	// Must not panic / leak goroutines when buffer wasn't wired.
	cmd.StartProgressFlusher(context.Background())
	cmd.StopProgressFlusher()
}

func TestCommandService_StartStopProgressFlusher_DrainsOnStop(t *testing.T) {
	t.Parallel()
	pr := newCountingProgressRepo()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	buf := appplayback.NewWatchProgressBuffer(pr, time.Hour)
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus, appplayback.WithWatchProgressBuffer(buf))

	ctx := context.Background()
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	cmd.StartProgressFlusher(ctx)
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 8000, false, ""))
	assert.Equal(t, int64(0), pr.Calls())

	cmd.StopProgressFlusher()
	assert.Equal(t, int64(1), pr.Calls(), "graceful stop drains the buffer")
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(8000), wp.Position.Milliseconds())
}

// panickingProgressRepo panics on the first UpsertMany and behaves normally
// afterwards - the shape of a driver that trips once (a nil map, a closed
// pool) rather than returning an error.
type panickingProgressRepo struct {
	*pbfakes.FakeWatchProgressRepository
	panicked atomic.Bool
	calls    atomic.Int64
}

func newPanickingProgressRepo() *panickingProgressRepo {
	return &panickingProgressRepo{FakeWatchProgressRepository: pbfakes.NewFakeWatchProgressRepository()}
}

// UpsertMany must be overridden (no virtual dispatch through embedding).
func (r *panickingProgressRepo) UpsertMany(ctx context.Context, entries []*playback.WatchProgress) error {
	r.calls.Add(1)
	if r.panicked.CompareAndSwap(false, true) {
		panic("simulated driver panic on the first flush")
	}
	return r.FakeWatchProgressRepository.UpsertMany(ctx, entries)
}

// "One bad flush must cost one flush" - the invariant safeFlush's own comment
// states. It matters because the goroutine-level recover in Start() would
// otherwise catch the panic and fall straight into the deferred final-flush
// and close(doneCh): the loop ends for the REST OF THE PROCESS LIFETIME,
// watch progress is silently never persisted again for any profile, pending
// grows unbounded, and Stop() still returns cleanly because doneCh is already
// closed. Nothing crashes, nothing logs an error the operator would connect
// to "my resume position stopped working".
//
// Every existing test in this file makes the repo return an ERROR; none makes
// it panic, so the recover that exists precisely for this was never exercised.
func TestWatchProgressBuffer_APanickingFlushCostsOnlyThatFlush(t *testing.T) {
	repo := newPanickingProgressRepo()
	b := appplayback.NewWatchProgressBuffer(repo, 10*time.Millisecond)
	b.Start(t.Context())

	// First tick: the flush panics and is contained.
	b.Enqueue(&playback.WatchProgress{
		ProfileID: shared.ProfileID("prf-1"), MediaItemID: shared.MediaItemID("mi-1"),
		Position: mustDurationMs(1000),
	})
	waitForCalls(t, &repo.calls, 1)

	// The loop must still be alive: a LATER write has to reach the repo.
	b.Enqueue(&playback.WatchProgress{
		ProfileID: shared.ProfileID("prf-2"), MediaItemID: shared.MediaItemID("mi-2"),
		Position: mustDurationMs(2000),
	})
	waitForCalls(t, &repo.calls, 2)

	b.Stop()
	stored, err := repo.GetByProfileAndMedia(context.Background(), "prf-2", "mi-2")
	require.NoError(t, err)
	require.NotNil(t, stored, "progress written after a panicking flush must still be persisted")
	assert.Equal(t, int64(2000), stored.Position.Milliseconds())
}

func waitForCalls(t *testing.T, calls *atomic.Int64, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d flush call(s), got %d - the flush loop died on the panic", want, calls.Load())
}
