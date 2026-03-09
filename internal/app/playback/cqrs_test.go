package playback_test

import (
	"context"
	"testing"

	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	"github.com/gravity-zero/evey/internal/domain/playback"
	pbfakes "github.com/gravity-zero/evey/internal/domain/playback/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCQRSServices() (
	*appplayback.CommandService,
	*appplayback.QueryService,
	*pbfakes.FakeWatchProgressRepository,
	*pbfakes.FakePlaySessionRepository,
	*userfakes.FakeUserRepository,
	*sharedfakes.FakeEventBus,
) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := appplayback.NewCommandService(progressRepo, sessionRepo, sessionRepo, userRepo, bus)
	q := appplayback.NewQueryService(progressRepo)
	return cmd, q, progressRepo, sessionRepo, userRepo, bus
}

func TestCommandService_ReportProgress_QueryService_Get(t *testing.T) {
	cmd, q, _, _, _, _ := newCQRSServices()
	ctx := context.Background()

	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m1", 45000, false))

	wp, err := q.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(45000), wp.PositionMs)
	assert.False(t, wp.Completed)
}

func TestCommandService_MarkAsWatched_QueryService_History(t *testing.T) {
	cmd, q, _, _, _, bus := newCQRSServices()
	ctx := context.Background()

	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))

	items, err := q.GetWatchHistory(ctx, "p1", 10, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.True(t, items[0].Completed)

	var found bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.marked.watched" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestCommandService_MarkAsUnwatched(t *testing.T) {
	cmd, q, _, _, _, _ := newCQRSServices()
	ctx := context.Background()

	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))
	require.NoError(t, cmd.MarkAsUnwatched(ctx, "p1", "m1"))

	wp, err := q.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed)
	assert.Equal(t, int64(0), wp.PositionMs)
}

func TestCommandService_StartStream_MaxStreams(t *testing.T) {
	cmd, _, _, _, userRepo, _ := newCQRSServices()
	ctx := context.Background()

	maxStreams := 1
	userRepo.Create(ctx, &user.User{ID: "u1", Username: "test", Enabled: true, MaxStreams: &maxStreams})

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.NoError(t, err)

	err = cmd.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrMaxStreams)
}

func TestCommandService_StopStream(t *testing.T) {
	cmd, _, _, sessionRepo, userRepo, bus := newCQRSServices()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "test", Enabled: true})

	session := &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}
	require.NoError(t, cmd.StartStream(ctx, session, "u1"))

	require.NoError(t, cmd.StopStream(ctx, "s1"))

	stored, err := sessionRepo.GetByID(ctx, "s1")
	require.NoError(t, err)
	assert.NotNil(t, stored.EndedAt)

	var stopped bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "playback.stopped" {
			stopped = true
		}
	}
	assert.True(t, stopped)
}

func TestCommandService_ReportProgressBySession(t *testing.T) {
	cmd, q, _, _, userRepo, _ := newCQRSServices()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "test", Enabled: true})

	session := &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}
	require.NoError(t, cmd.StartStream(ctx, session, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 55000))

	wp, err := q.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(55000), wp.PositionMs)
}

func TestQueryService_GetResumeItems(t *testing.T) {
	cmd, q, _, _, _, _ := newCQRSServices()
	ctx := context.Background()

	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m1", 30000, false))
	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m2", 0, false))

	items, err := q.GetResumeItems(ctx, "p1", 10)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "m1", items[0].MediaItemID)
}

func TestServiceWrapper_DelegatesToCQRS(t *testing.T) {
	svc, _, _, _, _ := newPlaybackSvc()
	ctx := context.Background()

	require.NoError(t, svc.ReportProgress(ctx, "p1", "m1", 12000, false))

	wp, err := svc.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(12000), wp.PositionMs)

	assert.NotNil(t, svc.CommandService())
	assert.NotNil(t, svc.QueryService())
}
