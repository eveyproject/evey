package playback_test

import (
	"context"
	"testing"

	appplayback "github.com/eveyproject/evey/internal/app/playback"
	"github.com/eveyproject/evey/internal/domain/playback"
	pbfakes "github.com/eveyproject/evey/internal/domain/playback/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCQRSServices() (
	*appplayback.CommandService,
	*appplayback.QueryService,
	*pbfakes.FakeWatchProgressRepository,
	*pbfakes.FakePlaySessionRepository,
	*userfakes.FakeUserRepository,
	*userfakes.FakeProfileRepository,
	*sharedfakes.FakeEventBus,
) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmd := appplayback.NewCommandService(progressRepo, sessionRepo, sessionRepo, userRepo, profileRepo, bus)
	q := appplayback.NewQueryService(progressRepo)
	return cmd, q, progressRepo, sessionRepo, userRepo, profileRepo, bus
}

func TestCommandService_ReportProgress_QueryService_Get(t *testing.T) {
	cmd, q, _, _, _, profileRepo, _ := newCQRSServices()
	ctx := context.Background()

	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m1", 45000, false))

	wp, err := q.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(45000), wp.Position.Milliseconds())
	assert.False(t, wp.Completed)
}

func TestCommandService_MarkAsWatched_QueryService_History(t *testing.T) {
	cmd, q, _, _, _, profileRepo, bus := newCQRSServices()
	ctx := context.Background()

	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
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
	cmd, q, _, _, _, profileRepo, _ := newCQRSServices()
	ctx := context.Background()

	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	require.NoError(t, cmd.MarkAsWatched(ctx, "p1", "m1"))
	require.NoError(t, cmd.MarkAsUnwatched(ctx, "p1", "m1"))

	// MarkAsUnwatched deletes the progress row; the query path surfaces
	// shared.ErrNotFound (the gRPC handler maps it to an empty proto).
	_, err := q.GetWatchProgress(ctx, "p1", "m1")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestCommandService_StartStream_MaxStreams(t *testing.T) {
	cmd, _, _, _, userRepo, profileRepo, _ := newCQRSServices()
	ctx := context.Background()

	maxStreams := 1
	userRepo.Create(ctx, &user.User{ID: "u1", Username: "test", Enabled: true, MaxStreams: &maxStreams})
	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.NoError(t, err)

	err = cmd.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrMaxStreams)
}

func TestCommandService_StopStream(t *testing.T) {
	cmd, _, _, sessionRepo, userRepo, profileRepo, bus := newCQRSServices()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "test", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

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
	cmd, q, _, sessionRepo, userRepo, profileRepo, _ := newCQRSServices()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "test", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	session := &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}
	require.NoError(t, cmd.StartStream(ctx, session, "u1"))
	require.NoError(t, cmd.ReportProgressBySession(ctx, "s1", 55000, false, ""))

	wp, err := q.GetWatchProgress(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.Equal(t, int64(55000), wp.Position.Milliseconds())

	stored, err := sessionRepo.GetByID(ctx, "s1")
	require.NoError(t, err)
	require.NotNil(t, stored.Position)
	assert.Equal(t, int64(55000), stored.Position.Milliseconds())
}

func TestQueryService_GetResumeItems(t *testing.T) {
	cmd, q, _, _, _, profileRepo, _ := newCQRSServices()
	ctx := context.Background()

	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})
	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m1", 30000, false))
	require.NoError(t, cmd.ReportProgress(ctx, "p1", "m2", 0, false))

	items, err := q.GetResumeItems(ctx, "p1", 10)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m1"), items[0].MediaItemID)
}

func TestCommandService_MarkAsWatched_ProfileNotFound(t *testing.T) {
	cmd, _, _, _, _, _, _ := newCQRSServices()
	ctx := context.Background()

	err := cmd.MarkAsWatched(ctx, "nonexistent", "m1")
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestCommandService_MarkAsUnwatched_ProfileNotFound(t *testing.T) {
	cmd, _, _, _, _, _, _ := newCQRSServices()
	ctx := context.Background()

	err := cmd.MarkAsUnwatched(ctx, "nonexistent", "m1")
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestCommandService_ReportProgress_ProfileNotFound(t *testing.T) {
	cmd, _, _, _, _, _, _ := newCQRSServices()
	ctx := context.Background()

	err := cmd.ReportProgress(ctx, "nonexistent", "m1", 5000, false)
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}

func TestCommandService_StartStream_ProfileNotFound(t *testing.T) {
	cmd, _, _, _, userRepo, _, _ := newCQRSServices()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "test", Enabled: true})

	err := cmd.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "nonexistent", MediaItemID: "m1"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrProfileNotFound)
}
