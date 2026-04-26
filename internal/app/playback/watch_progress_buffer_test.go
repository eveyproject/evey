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
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			PositionMs:   int64(i * 1000),
			LastPlayedAt: time.Now(),
		})
	}

	// 10 Enqueues, 0 Upserts — the hot path stays off the DB.
	assert.Equal(t, int64(0), repo.Calls())
	assert.Equal(t, 1, buf.Pending(), "same key should fold to a single entry")
}

func TestWatchProgressBuffer_FlushPersistsLatestPerKey(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour) // no auto-tick

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", PositionMs: 1000, LastPlayedAt: time.Now()})
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", PositionMs: 5000, LastPlayedAt: time.Now()})
	buf.Enqueue(&playback.WatchProgress{ProfileID: "p2", MediaItemID: "m2", PositionMs: 2000, LastPlayedAt: time.Now()})

	n := buf.Flush(context.Background())
	assert.Equal(t, 2, n)
	assert.Equal(t, 0, buf.Pending())

	wp1, err := repo.GetByProfileAndMedia(context.Background(), "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(5000), wp1.PositionMs, "latest position wins")

	wp2, err := repo.GetByProfileAndMedia(context.Background(), "p2", "m2")
	require.NoError(t, err)
	assert.Equal(t, int64(2000), wp2.PositionMs)
}

func TestWatchProgressBuffer_PeriodicFlush(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buf.Start(ctx)
	defer buf.Stop()

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", PositionMs: 1000, LastPlayedAt: time.Now()})

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

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", PositionMs: 9000, LastPlayedAt: time.Now()})
	require.Equal(t, int64(0), repo.Calls(), "ticker should not have fired yet")

	buf.Stop()

	assert.Equal(t, int64(1), repo.Calls(), "Stop must drain pending writes")
	wp, err := repo.GetByProfileAndMedia(context.Background(), "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(9000), wp.PositionMs)
}

func TestWatchProgressBuffer_CtxCancelTriggersFinalFlush(t *testing.T) {
	t.Parallel()
	repo := newCountingProgressRepo()
	buf := appplayback.NewWatchProgressBuffer(repo, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	buf.Start(ctx)

	buf.Enqueue(&playback.WatchProgress{ProfileID: "p1", MediaItemID: "m1", PositionMs: 4242, LastPlayedAt: time.Now()})
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
					PositionMs:   int64(w*1000 + i),
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
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 5000, false))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 10000, false))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 15000, false))

	assert.Equal(t, int64(0), pr.Calls(), "buffer should swallow upserts")

	// Manual flush persists the latest folded state.
	persisted := buf.Flush(ctx)
	assert.Equal(t, 1, persisted)
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(15000), wp.PositionMs, "latest position wins after fold")
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

	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 5000, false))
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
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 8000, false))
	assert.Equal(t, int64(0), pr.Calls())

	cmd.StopProgressFlusher()
	assert.Equal(t, int64(1), pr.Calls(), "graceful stop drains the buffer")
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(8000), wp.PositionMs)
}
