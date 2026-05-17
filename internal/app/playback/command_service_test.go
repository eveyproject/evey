package playback_test

import (
	"context"
	"errors"
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

func setupCmd() (
	*appplayback.CommandService,
	*pbfakes.FakeWatchProgressRepository,
	*pbfakes.FakePlaySessionRepository,
	*userfakes.FakeUserRepository,
	*userfakes.FakeProfileRepository,
	*sharedfakes.FakeEventBus,
) {
	pr := pbfakes.NewFakeWatchProgressRepository()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus)
	return cmd, pr, sr, ur, pfr, bus
}

func seedUserProfile(ctx context.Context, ur *userfakes.FakeUserRepository, pfr *userfakes.FakeProfileRepository, maxStreams *int) {
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true, MaxStreams: maxStreams})
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
}

// --- Mutation killers: event publish error must NOT propagate ---

func TestStartStream_InvalidSession(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	ctx := context.Background()

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "", MediaItemID: "m1"}, "u1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid play session")
}

func TestStartStream_PublishError_StillSucceeds(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, bus := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	bus.PublishErr = errors.New("bus down")
	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	assert.NoError(t, err)
}

func TestStopStream_PublishError_StillSucceeds(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, bus := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	bus.PublishErr = errors.New("bus down")
	err := cmd.StopStream(ctx, "s1")
	assert.NoError(t, err)
}

func TestMarkAsWatched_PublishError_StillSucceeds(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, pfr, bus := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	bus.PublishErr = errors.New("bus down")
	err := cmd.MarkAsWatched(ctx, "p1", "m1")
	assert.NoError(t, err)
}

// --- NOT COVERED paths ---

func TestStopStream_SessionNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	ctx := context.Background()

	err := cmd.StopStream(ctx, "nonexistent")
	assert.ErrorIs(t, err, appplayback.ErrSessionNotFound)
}

func TestReportProgressBySession_SessionNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	ctx := context.Background()

	err := cmd.ReportProgressBySession(ctx, "nonexistent", 5000, false)
	assert.ErrorIs(t, err, appplayback.ErrSessionNotFound)
}

func TestStartStream_UserNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, pfr, _ := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get user")
}

func TestStartStream_MaxStreamsNil_NoLimit(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	for i := range 5 {
		err := cmd.StartStream(ctx, &playback.PlaySession{
			ID: "s" + string(rune('0'+i)), ProfileID: "p1", MediaItemID: "m1",
		}, "u1")
		require.NoError(t, err)
	}
}

func TestStartStream_MaxStreamsZero_NoLimit(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	zero := 0
	seedUserProfile(ctx, ur, pfr, &zero)

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.NoError(t, err)
	err = cmd.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1")
	require.NoError(t, err)
}

func TestStartStream_ExactlyAtMax_Allowed(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	max := 2
	seedUserProfile(ctx, ur, pfr, &max)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1"))

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s3", ProfileID: "p1", MediaItemID: "m3"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrMaxStreams)
}

func TestStartStream_SetsStartedAt(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	before := time.Now()
	session := &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}
	require.NoError(t, cmd.StartStream(ctx, session, "u1"))

	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.False(t, stored.StartedAt.Before(before))
}

func TestStartStream_PublishesStartedEvent(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, bus := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{
		ID: "s1", ProfileID: "p1", MediaItemID: "m1", PlayMethod: "direct_play",
	}, "u1"))

	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "playback.started", events[0].EventName())
	started := events[0].(playback.PlaybackStarted)
	assert.Equal(t, "s1", started.SessionID)
	assert.Equal(t, shared.ProfileID("p1"), started.ProfileID)
	assert.Equal(t, shared.MediaItemID("m1"), started.MediaItemID)
	assert.Equal(t, "direct_play", started.PlayMethod)
}

func TestStopStream_SetsEndedAt(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	before := time.Now()
	require.NoError(t, cmd.StopStream(ctx, "s1"))

	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, stored.EndedAt)
	assert.False(t, stored.EndedAt.Before(before))
}

func TestStopStream_PublishesStoppedEvent(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, bus := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.StopStream(ctx, "s1"))

	var found *playback.PlaybackStopped
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "playback.stopped" {
			stopped := e.(playback.PlaybackStopped)
			found = &stopped
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, "s1", found.SessionID)
	assert.Equal(t, shared.ProfileID("p1"), found.ProfileID)
	assert.Equal(t, shared.MediaItemID("m1"), found.MediaItemID)
}

func TestReportProgressBySession_UpdatesSessionPosition(t *testing.T) {
	t.Parallel()
	cmd, pr, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 42000, false))

	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, stored.Position)
	assert.Equal(t, int64(42000), stored.Position.Milliseconds())

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(42000), wp.Position.Milliseconds())
	assert.False(t, wp.Completed)
}

// fakeDurationGetter satisfies appplayback.MediaItemDurationGetter so the
// auto-complete-at-90% path can be exercised without dragging the library
// query service into the playback test setup.
type fakeDurationGetter struct {
	durations map[string]int64
	err       error
}

func (f *fakeDurationGetter) GetMediaItemDurationMs(_ context.Context, mediaItemID string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.durations[mediaItemID], nil
}

func setupCmdWithDurations(durations map[string]int64) (
	*appplayback.CommandService,
	*pbfakes.FakeWatchProgressRepository,
	*pbfakes.FakePlaySessionRepository,
	*userfakes.FakeUserRepository,
	*userfakes.FakeProfileRepository,
) {
	pr := pbfakes.NewFakeWatchProgressRepository()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	getter := &fakeDurationGetter{durations: durations}
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus, appplayback.WithMediaItemDurationGetter(getter))
	return cmd, pr, sr, ur, pfr
}

func TestReportProgressBySession_AutoCompletesAt90Percent(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": 100_000})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// 89% — must NOT auto-complete (just under the 90% threshold).
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 89_000, false))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed, "position 89%% must stay incomplete")

	// 90% exactly — flips the flag.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 90_000, false))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed, "position 90%% must auto-complete")
}

func TestReportProgressBySession_CompletedStickyAcrossSeekBack(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": 100_000})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// Cross threshold so Completed becomes true.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 95_000, false))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed)

	// User seeks back to 20% — repo's sticky-true semantic keeps Completed.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 20_000, false))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed, "seek-back must NOT regress completed=true")
	assert.Equal(t, int64(20_000), wp.Position.Milliseconds())
}

func TestReportProgressBySession_NoGetter_NoAutoComplete(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr, _ := setupCmd() // no duration getter wired
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 999_999_999, false))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed, "without a duration source the legacy behaviour stays")
}

func TestReportProgressBySession_GetterReturnsZero_NoAutoComplete(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": 0})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 60_000, false))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed, "duration=0 means unknown, never auto-complete")
}

func TestReportProgressBySession_GetterError_DegradesGracefully(t *testing.T) {
	t.Parallel()
	pr := pbfakes.NewFakeWatchProgressRepository()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	getter := &fakeDurationGetter{err: errors.New("db blip")}
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus, appplayback.WithMediaItemDurationGetter(getter))
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	// Must succeed even when the duration lookup errors — auto-complete is
	// best-effort, never blocks the progress write.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 95_000, false))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed)
}

func TestMarkAsWatched_StaysStickyAgainstSubsequentProgressTick(t *testing.T) {
	t.Parallel()
	// Regression: the historical hardcoded Completed=false in
	// ReportProgressBySession used to silently undo MarkAsWatched on the
	// very next player tick. Sticky-true Upsert + per-position computation
	// fix it. This test guards against re-introducing the bug.
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": 100_000})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))

	// Next tick lands well below 90% — must NOT regress to false.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 10_000, false))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed, "MarkAsWatched must survive a subsequent progress tick at <90%%")
}

func TestGetSession(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	got, err := cmd.GetSession(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", got.ID)

	_, err = cmd.GetSession(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestCleanupStaleSessions(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	staleSession := &playback.PlaySession{
		ID: "s-stale", ProfileID: "p1", MediaItemID: "m1",
		StartedAt: time.Now().Add(-3 * time.Hour),
	}
	sr.Create(ctx, staleSession)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s-fresh", ProfileID: "p1", MediaItemID: "m2"}, "u1"))

	n, err := cmd.CleanupStaleSessions(ctx, 2*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	stale, _ := sr.GetByID(ctx, "s-stale")
	assert.NotNil(t, stale.EndedAt)

	fresh, _ := sr.GetByID(ctx, "s-fresh")
	assert.Nil(t, fresh.EndedAt)
}

func TestMarkAsWatched_SetsCompleted(t *testing.T) {
	t.Parallel()
	cmd, pr, _, _, pfr, bus := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed)
	assert.Equal(t, 1, wp.PlayCount)

	var found bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.marked.watched" {
			watched := e.(playback.MediaMarkedWatched)
			assert.Equal(t, shared.ProfileID("p1"), watched.ProfileID)
			assert.Equal(t, shared.MediaItemID("m1"), watched.MediaItemID)
			found = true
		}
	}
	assert.True(t, found)
}

func TestMarkAsUnwatched_DeletesProgressRow(t *testing.T) {
	t.Parallel()
	cmd, pr, _, _, pfr, _ := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))
	require.NoError(t, cmd.MarkAsUnwatched(ctx, "p1", "m1"))

	// MarkAsUnwatched deletes the row entirely — the item appears
	// never-watched again, matching the Jellyfin semantics the gRPC
	// handler relies on (NotFound → empty WatchProgress).
	_, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestMarkAsUnwatched_ProfileNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	err := cmd.MarkAsUnwatched(context.Background(), "nonexistent", "m1")
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestReportProgress_ProfileNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	err := cmd.ReportProgress(context.Background(), "nonexistent", "m1", 5000, false)
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestStartStream_CreateSessionError(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	sr.Err = errors.New("db error")
	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create session")
}

func TestStopStream_UpdateError(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	sr.Err = errors.New("db error")
	err := cmd.StopStream(ctx, "s1")
	assert.Error(t, err)
}

func TestReportProgressBySession_UpdateError(t *testing.T) {
	t.Parallel()
	pr := pbfakes.NewFakeWatchProgressRepository()
	sr := pbfakes.NewFakePlaySessionRepository()
	ur := userfakes.NewFakeUserRepository()
	pfr := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := appplayback.NewCommandService(pr, sr, sr, ur, pfr, bus)
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// set error after session created so GetByID still works but Update fails
	// The fake checks Err before lock and after lock, so Update will fail
	// But GetByID also checks Err. We need a different approach:
	// delete the session so Update returns "not found" which wraps as "update session position"
	// Actually, let's just set progress repo error to test the downstream call
	pr.Err = errors.New("db error")
	err := cmd.ReportProgressBySession(ctx, "s1", 5000, false)
	assert.Error(t, err)
}

func TestStartStream_CountActiveError(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	max := 2
	seedUserProfile(ctx, ur, pfr, &max)

	sr.Err = errors.New("db error")
	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestStartCleanup_RunsInitialCleanup(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	stale := &playback.PlaySession{
		ID: "s-stale", ProfileID: "p1", MediaItemID: "m1",
		StartedAt: time.Now().Add(-3 * time.Hour),
	}
	sr.Create(ctx, stale)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd.StartCleanupWithInterval(cancelCtx, 10*time.Millisecond, 2*time.Hour)

	// Wait deterministically for the cleanup goroutine to close the stale
	// session. Replaces a fragile time.Sleep that raced on slow CI/WSL.
	require.Eventually(t, func() bool {
		stored, err := sr.GetByID(ctx, "s-stale")
		return err == nil && stored != nil && stored.EndedAt != nil
	}, 2*time.Second, 5*time.Millisecond)
}

func TestStartCleanup_TickerCleansNewStale(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd.StartCleanupWithInterval(cancelCtx, 20*time.Millisecond, 1*time.Hour)

	sr.Create(ctx, &playback.PlaySession{
		ID: "s-late", ProfileID: "p1", MediaItemID: "m1",
		StartedAt: time.Now().Add(-2 * time.Hour),
	})

	// A subsequent ticker tick must close the new stale session. Polling
	// avoids racing the ticker on slow hosts.
	require.Eventually(t, func() bool {
		stored, err := sr.GetByID(ctx, "s-late")
		return err == nil && stored != nil && stored.EndedAt != nil
	}, 2*time.Second, 5*time.Millisecond)
}

func TestMarkAsWatched_UpsertError(t *testing.T) {
	t.Parallel()
	cmd, pr, _, _, pfr, _ := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	pr.Err = errors.New("db error")
	err := cmd.MarkAsWatched(ctx, "p1", "m1")
	assert.Error(t, err)
}

func TestMarkAsWatched_ProfileNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	err := cmd.MarkAsWatched(context.Background(), "nonexistent", "m1")
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestReportProgress_HappyPath(t *testing.T) {
	t.Parallel()
	cmd, pr, _, _, pfr, _ := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m1", 12000, true))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(12000), wp.Position.Milliseconds())
	assert.True(t, wp.Completed)
	assert.Equal(t, shared.ProfileID("p1"), wp.ProfileID)
	assert.Equal(t, shared.MediaItemID("m1"), wp.MediaItemID)
}

func TestMarkAsUnwatched_UpsertError(t *testing.T) {
	t.Parallel()
	cmd, pr, _, _, pfr, _ := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	pr.Err = errors.New("db error")
	err := cmd.MarkAsUnwatched(ctx, "p1", "m1")
	assert.Error(t, err)
}

func TestReportProgress_UpsertError(t *testing.T) {
	t.Parallel()
	cmd, pr, _, _, pfr, _ := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	pr.Err = errors.New("db error")
	err := cmd.ReportProgress(ctx, "p1", "m1", 5000, false)
	assert.Error(t, err)
}

func TestStartStream_ProfileNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, _, _ := setupCmd()
	ctx := context.Background()
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}
