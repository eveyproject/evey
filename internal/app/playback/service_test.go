package playback_test

import (
	"context"
	"testing"

	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	"github.com/gravity-zero/evey/internal/domain/playback"
	pbfakes "github.com/gravity-zero/evey/internal/domain/playback/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPlaybackSvc() (*appplayback.Service, *pbfakes.FakeWatchProgressRepository, *pbfakes.FakePlaySessionRepository, *userfakes.FakeUserRepository, *sharedfakes.FakeEventBus) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplayback.NewService(progressRepo, sessionRepo, userRepo, bus)
	return svc, progressRepo, sessionRepo, userRepo, bus
}

func TestStartStream_MaxStreams(t *testing.T) {
	svc, _, _, userRepo, _ := newPlaybackSvc()

	maxStreams := 1
	userRepo.Create(context.Background(), &user.User{
		ID:         "u1",
		Username:   "test",
		Enabled:    true,
		MaxStreams: &maxStreams,
	})

	ctx := context.Background()

	err := svc.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.NoError(t, err)

	err = svc.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrMaxStreams)
}

func TestUpdateProgress(t *testing.T) {
	svc, progressRepo, _, userRepo, _ := newPlaybackSvc()

	userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "alice", Enabled: true})

	ctx := context.Background()
	err := svc.ReportProgress(ctx, "prof-1", "item-1", 30000, false)
	require.NoError(t, err)

	wp, err := progressRepo.GetByProfileAndMedia(ctx, "prof-1", "item-1")
	require.NoError(t, err)
	assert.Equal(t, int64(30000), wp.PositionMs)
	assert.False(t, wp.Completed)

	// update progress further
	err = svc.ReportProgress(ctx, "prof-1", "item-1", 60000, true)
	require.NoError(t, err)

	wp, err = progressRepo.GetByProfileAndMedia(ctx, "prof-1", "item-1")
	require.NoError(t, err)
	assert.Equal(t, int64(60000), wp.PositionMs)
	assert.True(t, wp.Completed)
}

func TestStartSession(t *testing.T) {
	svc, _, sessionRepo, userRepo, bus := newPlaybackSvc()

	userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "alice", Enabled: true})

	ctx := context.Background()
	session := &playback.PlaySession{
		ID:          "sess-1",
		ProfileID:   "prof-1",
		MediaItemID: "item-1",
		PlayMethod:  string(playback.DirectPlay),
	}
	err := svc.StartStream(ctx, session, "u1")
	require.NoError(t, err)

	stored, err := sessionRepo.GetByID(ctx, "sess-1")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", stored.ID)
	assert.False(t, stored.StartedAt.IsZero())

	var started bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "playback.started" {
			started = true
		}
	}
	assert.True(t, started)
}

func TestStopSession(t *testing.T) {
	svc, _, sessionRepo, userRepo, bus := newPlaybackSvc()

	userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "alice", Enabled: true})

	ctx := context.Background()
	session := &playback.PlaySession{
		ID:          "sess-1",
		ProfileID:   "prof-1",
		MediaItemID: "item-1",
	}
	err := svc.StartStream(ctx, session, "u1")
	require.NoError(t, err)

	err = svc.StopStream(ctx, "sess-1")
	require.NoError(t, err)

	stored, err := sessionRepo.GetByID(ctx, "sess-1")
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
