package playback_test

import (
	"context"
	"errors"
	"math"
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

func TestStartStream_Idempotent_DedupsDoubleClick(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, bus := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	first := &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1", DeviceName: "tab"}
	require.NoError(t, cmd.StartStream(ctx, first, "u1"))

	// A double-click / retry mints a fresh server-side ID for the same
	// (profile, media, device); it must resolve to the live session.
	second := &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m1", DeviceName: "tab"}
	require.NoError(t, cmd.StartStream(ctx, second, "u1"))

	assert.Equal(t, "s1", second.ID, "double-click must reuse the existing session")
	active, _ := sr.GetActiveByProfile(ctx, "p1")
	assert.Len(t, active, 1, "no phantom session created")

	var started int
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == (playback.PlaybackStarted{}).EventName() {
			started++
		}
	}
	assert.Equal(t, 1, started, "PlaybackStarted must not be re-emitted on the dedup")
}

func TestStartStream_DifferentDevice_NotDeduped(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1", DeviceName: "tab"}, "u1"))
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m1", DeviceName: "tv"}, "u1"))

	active, _ := sr.GetActiveByProfile(ctx, "p1")
	assert.Len(t, active, 2, "same media on a different device is a distinct session")
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

	err := cmd.ReportProgressBySession(ctx, "nonexistent", 5000, false, "")
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

func TestStartStream_MaxDevicesPerProfile_Enforced(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	limit := 2
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true, MaxDevicesPerProfile: &limit})
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1"))

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s3", ProfileID: "p1", MediaItemID: "m3"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrMaxDevicesPerProfile)
}

func TestStartStream_DeviceLimitOnly_AcquiresStreamLock(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	limit := 2
	// Device limit but NO stream limit: the create path must still take the
	// per-user advisory lock so concurrent starts can't race past the device cap.
	// Pre-fix the lock was acquired only in the stream-limit branch, so LockCalls
	// here would have been 0.
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true, MaxDevicesPerProfile: &limit})
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	assert.GreaterOrEqual(t, sr.LockCalls, 1, "device-limited start must acquire the per-user stream lock")
}

func TestStartStream_MaxDevicesNil_NoLimit(t *testing.T) {
	t.Parallel()
	cmd, _, _, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	ur.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	for i := range 5 {
		err := cmd.StartStream(ctx, &playback.PlaySession{
			ID: "s" + string(rune('0'+i)), ProfileID: "p1", MediaItemID: "m1",
		}, "u1")
		require.NoError(t, err)
	}
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
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 42000, false, ""))

	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, stored.Position)
	assert.Equal(t, int64(42000), stored.Position.Milliseconds())

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(42000), wp.Position.Milliseconds())
	assert.False(t, wp.Completed)
}

// longTestDurationMs is comfortably above MinAutoResumeDurationMs (300_000)
// so threshold tests are not also gated by the short-item floor. Positions
// inside tests scale relative to this - a 10% increment is longTestPctMs.
const (
	longTestDurationMs int64 = 1_000_000 // 1000 s ≈ 16.6 min
	longTestPctMs      int64 = longTestDurationMs / 100
)

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
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// 89% - must NOT auto-complete (just under the 90% threshold).
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 89*longTestPctMs, false, ""))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed, "position 89%% must stay incomplete")

	// 90% exactly - flips the flag.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 90*longTestPctMs, false, ""))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed, "position 90%% must auto-complete")
}

func TestReportProgressBySession_CompletedStickyAcrossSeekBack(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// Cross threshold so Completed becomes true.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 95*longTestPctMs, false, ""))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed)

	// User seeks back to 20% - repo's sticky-true semantic keeps Completed.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 20*longTestPctMs, false, ""))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed, "seek-back must NOT regress completed=true")
	assert.Equal(t, 20*longTestPctMs, wp.Position.Milliseconds())
}

func TestReportProgressBySession_NoGetter_NoAutoComplete(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr, _ := setupCmd() // no duration getter wired
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 999_999_999, false, ""))

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
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 60_000, false, ""))

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
	// Must succeed even when the duration lookup errors - auto-complete is
	// best-effort, never blocks the progress write.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 95_000, false, ""))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed)
}

func TestReportProgressBySession_OverflowPosition_CompletesNotGarbage(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// A forged position that overflows positionMs*Denominator (int64). It is far
	// beyond the duration, so the completion check must short-circuit to true
	// instead of computing a garbage result from the wrapped multiply.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", math.MaxInt64/5, false, ""))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed, "a position past the end must complete, not overflow to garbage")
}

func TestReportProgress_AutoCompletesAt90Percent(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	// Caller sends completed=false but the position crosses the threshold -
	// the server-side getter promotes it to true. Matches the BySession
	// path so CLI / non-web clients get parity with mobile/web.
	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m1", 95*longTestPctMs, false))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed)
}

func TestReportProgress_ExplicitCompletedAlwaysWins(t *testing.T) {
	t.Parallel()
	// Even when position is below the auto-complete threshold, an explicit
	// completed=true from the caller still flips the flag (caller intent).
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m1", 1_000, true))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed)
}

func TestReportProgressBySession_ShortItemMidStream_NoProgressRow(t *testing.T) {
	t.Parallel()
	// 2-minute trailer: a pause at 0:15 must NOT clutter "Continue Watching"
	// with a 2-minute-trailer entry (the 300 s floor, see MinAutoResumeDurationMs).
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"trailer": 120_000})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "trailer"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 15_000, false, ""))

	_, err := pr.GetByProfileAndMedia(ctx, "p1", "trailer")
	assert.ErrorIs(t, err, shared.ErrNotFound, "short-item mid-stream tick must not persist a watch_progress row")
}

func TestReportProgressBySession_ShortItemCompleted_PersistsAndCounts(t *testing.T) {
	t.Parallel()
	// Same short trailer, but the user watched it to the end: completion
	// IS written (and counted) - the floor only suppresses mid-stream noise.
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"trailer": 120_000})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "trailer"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 115_000, false, ""))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "trailer")
	require.NoError(t, err)
	assert.True(t, wp.Completed)
	assert.Equal(t, 1, wp.PlayCount)
}

func TestReportProgress_ShortItemMidStream_NoProgressRow(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"trailer": 120_000})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	// Caller-supplied completed=false, position 0:15 of 2min trailer.
	require.NoError(t, cmd.ReportProgress(ctx, "p1", "trailer", 15_000, false))

	_, err := pr.GetByProfileAndMedia(ctx, "p1", "trailer")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestReportProgress_ShortItemExplicitCompletedWritesThrough(t *testing.T) {
	t.Parallel()
	// Explicit completed=true from the caller bypasses the floor - the
	// short-item suppression only applies to inferred mid-stream ticks.
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"trailer": 120_000})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.ReportProgress(ctx, "p1", "trailer", 5_000, true))

	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "trailer")
	require.NoError(t, err)
	assert.True(t, wp.Completed)
}

func TestReportProgressBySession_PlayCountIncrementsOnFirstCompletion(t *testing.T) {
	t.Parallel()
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))

	// First crossing of the threshold: play_count bumps from 0 to 1.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 92*longTestPctMs, false, ""))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed)
	assert.Equal(t, 1, wp.PlayCount, "first completion must bump play_count to 1")

	// Subsequent ticks while still completed must NOT keep bumping the
	// counter - the sticky-true gate keeps EXCLUDED.completed=true but
	// the previous row already has completed=true, so the transition
	// branch doesn't fire. Re-watching from scratch is a separate flow
	// (Delete via MarkAsUnwatched, then new completion).
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 98*longTestPctMs, false, ""))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, 1, wp.PlayCount, "follow-up ticks past 90% must NOT keep incrementing")
}

func TestReportProgressBySession_PlayCountPreservedBetweenSessions(t *testing.T) {
	t.Parallel()
	// Regression: the pre-fix Upsert wrote PlayCount=0 on every tick,
	// overwriting MarkAsWatched's PlayCount=1 silently. Sticky-max +
	// transition-bump fixes it.
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, 1, wp.PlayCount)

	// Mid-stream progress tick at 20% - must NOT regress PlayCount to 0.
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 20*longTestPctMs, false, ""))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, 1, wp.PlayCount, "progress tick must NOT regress play_count from MarkAsWatched")
}

func TestMarkAsWatched_StaysStickyAgainstSubsequentProgressTick(t *testing.T) {
	t.Parallel()
	// Regression: the historical hardcoded Completed=false in
	// ReportProgressBySession used to silently undo MarkAsWatched on the
	// very next player tick. Sticky-true Upsert + per-position computation
	// fix it. This test guards against re-introducing the bug.
	cmd, pr, _, ur, pfr := setupCmdWithDurations(map[string]int64{"m1": longTestDurationMs})
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))

	// Next tick lands well below 90% - must NOT regress to false.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 10*longTestPctMs, false, ""))
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

	// MarkAsUnwatched deletes the row entirely - the item appears
	// never-watched again, matching the semantics the gRPC
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

func TestClearWatchHistory_DeletesAllForProfileOnly(t *testing.T) {
	t.Parallel()
	cmd, pr, _, _, pfr, _ := setupCmd()
	ctx := context.Background()
	pfr.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	pfr.Create(ctx, &user.Profile{ID: "p2", UserID: "u1"})

	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))
	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m2"))
	require.NoError(t, cmd.MarkAsWatched(ctx, "p2", "m3"))

	deleted, err := cmd.ClearWatchHistory(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	_, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.ErrorIs(t, err, shared.ErrNotFound)
	_, err = pr.GetByProfileAndMedia(ctx, "p1", "m2")
	require.ErrorIs(t, err, shared.ErrNotFound)
	// Another profile's history is untouched.
	_, err = pr.GetByProfileAndMedia(ctx, "p2", "m3")
	require.NoError(t, err)
}

func TestClearWatchHistory_ProfileNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	_, err := cmd.ClearWatchHistory(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestReportProgress_ProfileNotFound(t *testing.T) {
	t.Parallel()
	cmd, _, _, _, _, _ := setupCmd()
	err := cmd.ReportProgress(context.Background(), "nonexistent", "m1", 5000, false)
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestStartStream_RepoError_Aborts(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	// Even with no caps, the claim now takes the advisory lock and lists active
	// sessions (for the idempotency dedup) before creating. A repo failure on
	// any of those steps must abort the start rather than create a session.
	sr.Err = errors.New("db error")
	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
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
	err := cmd.ReportProgressBySession(ctx, "s1", 5000, false, "")
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

// Cast relay: while a web sender drives a Chromecast, each progress tick
// carries the REMOTE device's name; the session records it (for the admin
// Now Playing panel) without touching the session's own device identity,
// and a tick without it (cast ended) clears it.
func TestReportProgressBySession_CastDeviceRelay(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{
		ID: "s1", ProfileID: "p1", MediaItemID: "m1",
		DeviceName: "Web Browser", DeviceType: "browser",
	}, "u1"))

	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 42000, false, "Salon TV"))
	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, "Salon TV", stored.CastDeviceName)
	assert.Equal(t, "Web Browser", stored.DeviceName, "the session's own device identity stays intact")

	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 45000, false, ""))
	stored, err = sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.Empty(t, stored.CastDeviceName, "cast end clears the relay")
}

// A progress tick that lands AFTER StopStream must not resurrect the stopped
// session: the guarded UpdateProgress never rewrites ended_at, so the session
// stays closed and drops out of the active-session views (the "Now Playing
// ghost after stop" + phantom quota bug the audit traced to a lost-update race
// on the normal player-close path). Regression guard for the resurrection.
func TestReportProgressAfterStop_DoesNotResurrectSession(t *testing.T) {
	t.Parallel()
	cmd, _, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.StopStream(ctx, "s1"))

	// The player-close cleanup fires a final progress report after stop.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 123456, false, ""))

	// The session must remain ended: no active session, zero quota usage.
	active, err := sr.ListActiveSessions(ctx)
	require.NoError(t, err)
	assert.Empty(t, active, "a stopped session must not reappear as active after a late progress tick")

	n, err := sr.CountActiveByUser(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, 0, n, "a resurrected ghost would inflate the active-stream quota")

	got, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.NotNil(t, got.EndedAt, "ended_at must stay set")
}

// A player that never gets its picture going still ticks, at position 0, and
// the freshest write wins: a failed start (codec refused, network gone) erased
// the resume point. Measured in prod on 2026-09-23 - a start that stayed at
// readyState 0 sent five ticks at 0 and an 8-minute resume point became 0.
// Until a session has reported a positive position, a 0 is not a position.
func TestReportProgressBySession_ZeroBeforeAnyPositionKeepsTheResumePoint(t *testing.T) {
	t.Parallel()
	cmd, pr, sr, ur, pfr, _ := setupCmd()
	ctx := context.Background()
	seedUserProfile(ctx, ur, pfr, nil)

	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s0", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s0", 480_000, false, ""))
	require.NoError(t, cmd.StopStream(ctx, "s0"))

	// A new session that never starts: ticks at 0.
	require.NoError(t, cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1"))
	for range 3 {
		require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 0, false, ""))
	}
	wp, err := pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(480_000), wp.Position.Milliseconds(), "a start that never played must not erase the resume point")
	stored, err := sr.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.False(t, stored.IsPaused, "the session tick itself is still recorded")

	// Starting over is still a choice: the first real position replaces it.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 3_000, false, ""))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(3_000), wp.Position.Milliseconds())

	// Once the session has played, a seek back to the very start is kept.
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 0, false, ""))
	wp, err = pr.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), wp.Position.Milliseconds())
}
