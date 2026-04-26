package playback_test

import (
	"context"
	"testing"

	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/domain/playback"
	pbfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPlaybackSvc() (*appplayback.Service, *pbfakes.FakeWatchProgressRepository, *pbfakes.FakePlaySessionRepository, *userfakes.FakeUserRepository, *userfakes.FakeProfileRepository, *sharedfakes.FakeEventBus) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplayback.NewService(progressRepo, progressRepo, sessionRepo, sessionRepo, userRepo, profileRepo, bus)
	return svc, progressRepo, sessionRepo, userRepo, profileRepo, bus
}

func TestStartStream_MaxStreams(t *testing.T) {
	t.Parallel()
	svc, _, _, userRepo, profileRepo, _ := newPlaybackSvc()
	ctx := context.Background()

	maxStreams := 1
	userRepo.Create(ctx, &user.User{
		ID:         "u1",
		Username:   "test",
		Enabled:    true,
		MaxStreams: &maxStreams,
	})
	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	err := svc.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.NoError(t, err)

	err = svc.StartStream(ctx, &playback.PlaySession{ID: "s2", ProfileID: "p1", MediaItemID: "m2"}, "u1")
	assert.ErrorIs(t, err, appplayback.ErrMaxStreams)
}

func TestUpdateProgress(t *testing.T) {
	t.Parallel()
	svc, progressRepo, _, userRepo, profileRepo, _ := newPlaybackSvc()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "prof-1", UserID: "u1"})

	err := svc.ReportProgress(ctx, "prof-1", "item-1", 30000, false)
	require.NoError(t, err)

	wp, err := progressRepo.GetByProfileAndMedia(ctx, "prof-1", "item-1")
	require.NoError(t, err)
	assert.Equal(t, int64(30000), wp.Position.Milliseconds())
	assert.False(t, wp.Completed)

	// update progress further
	err = svc.ReportProgress(ctx, "prof-1", "item-1", 60000, true)
	require.NoError(t, err)

	wp, err = progressRepo.GetByProfileAndMedia(ctx, "prof-1", "item-1")
	require.NoError(t, err)
	assert.Equal(t, int64(60000), wp.Position.Milliseconds())
	assert.True(t, wp.Completed)
}

func TestStartSession(t *testing.T) {
	t.Parallel()
	svc, _, sessionRepo, userRepo, profileRepo, bus := newPlaybackSvc()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "prof-1", UserID: "u1"})

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
	t.Parallel()
	svc, _, sessionRepo, userRepo, profileRepo, bus := newPlaybackSvc()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "prof-1", UserID: "u1"})

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

func TestServiceWrapper_ReportProgressBySession(t *testing.T) {
	t.Parallel()
	svc, _, _, userRepo, profileRepo, _ := newPlaybackSvc()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	err := svc.StartStream(ctx, &playback.PlaySession{ID: "s1", ProfileID: "p1", MediaItemID: "m1"}, "u1")
	require.NoError(t, err)

	err = svc.ReportProgressBySession(ctx, "s1", 50000, false)
	require.NoError(t, err)
}

func TestServiceWrapper_GetWatchHistory(t *testing.T) {
	t.Parallel()
	svc, _, _, userRepo, profileRepo, _ := newPlaybackSvc()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, svc.ReportProgress(ctx, "p1", "m1", 10000, true))

	items, err := svc.GetWatchHistory(ctx, "p1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestServiceWrapper_GetResumeItems(t *testing.T) {
	t.Parallel()
	svc, _, _, userRepo, profileRepo, _ := newPlaybackSvc()
	ctx := context.Background()

	userRepo.Create(ctx, &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, svc.ReportProgress(ctx, "p1", "m1", 30000, false))

	items, err := svc.GetResumeItems(ctx, "p1", 10)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestServiceWrapper_GetNextUp(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newPlaybackSvc()
	ctx := context.Background()

	items, err := svc.GetNextUp(ctx, "p1", 10)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestServiceWrapper_MarkAsWatched(t *testing.T) {
	t.Parallel()
	svc, progressRepo, _, _, profileRepo, _ := newPlaybackSvc()
	ctx := context.Background()

	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	err := svc.MarkAsWatched(ctx, "p1", "m1")
	require.NoError(t, err)

	wp, err := progressRepo.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.True(t, wp.Completed)
}

func TestServiceWrapper_MarkAsUnwatched(t *testing.T) {
	t.Parallel()
	svc, progressRepo, _, _, profileRepo, _ := newPlaybackSvc()
	ctx := context.Background()

	profileRepo.Create(ctx, &user.Profile{ID: "p1", UserID: "u1"})

	require.NoError(t, svc.MarkAsWatched(ctx, "p1", "m1"))
	err := svc.MarkAsUnwatched(ctx, "p1", "m1")
	require.NoError(t, err)

	wp, err := progressRepo.GetByProfileAndMedia(ctx, "p1", "m1")
	require.NoError(t, err)
	assert.False(t, wp.Completed)
}

func TestGetActiveSessions_Empty(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newPlaybackSvc()
	ctx := context.Background()

	sessions, err := svc.GetActiveSessions(ctx)
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestGetActiveSessions_ReturnsActive(t *testing.T) {
	t.Parallel()
	svc, _, _, userRepo, profileRepo, _ := newPlaybackSvc()
	userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(context.Background(), &user.Profile{ID: "p1", UserID: "u1", Name: "default"})

	ctx := context.Background()
	err := svc.StartStream(ctx, &playback.PlaySession{
		ID: "s1", ProfileID: "p1", MediaItemID: "m1",
		PlayMethod: string(playback.DirectPlay),
		DeviceName: "Chrome", DeviceType: "browser",
	}, "u1")
	require.NoError(t, err)

	err = svc.StartStream(ctx, &playback.PlaySession{
		ID: "s2", ProfileID: "p1", MediaItemID: "m2",
		PlayMethod: string(playback.Transcode),
		VideoCodec: "hevc",
	}, "u1")
	require.NoError(t, err)

	sessions, err := svc.GetActiveSessions(ctx)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestGetActiveSessions_ExcludesStopped(t *testing.T) {
	t.Parallel()
	svc, _, _, userRepo, profileRepo, _ := newPlaybackSvc()
	userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "bob", Enabled: true})
	profileRepo.Create(context.Background(), &user.Profile{ID: "p1", UserID: "u1", Name: "default"})

	ctx := context.Background()
	err := svc.StartStream(ctx, &playback.PlaySession{
		ID: "s1", ProfileID: "p1", MediaItemID: "m1",
	}, "u1")
	require.NoError(t, err)

	err = svc.StartStream(ctx, &playback.PlaySession{
		ID: "s2", ProfileID: "p1", MediaItemID: "m2",
	}, "u1")
	require.NoError(t, err)

	err = svc.StopStream(ctx, "s1")
	require.NoError(t, err)

	sessions, err := svc.GetActiveSessions(ctx)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "s2", sessions[0].SessionID)
}

func TestGetActiveSessions_TranscodeInfo(t *testing.T) {
	t.Parallel()
	svc, _, _, userRepo, profileRepo, _ := newPlaybackSvc()
	userRepo.Create(context.Background(), &user.User{ID: "u1", Username: "alice", Enabled: true})
	profileRepo.Create(context.Background(), &user.Profile{ID: "p1", UserID: "u1", Name: "default"})

	ctx := context.Background()
	err := svc.StartStream(ctx, &playback.PlaySession{
		ID: "s1", ProfileID: "p1", MediaItemID: "m1",
		PlayMethod: string(playback.Transcode),
		VideoCodec: "hevc",
	}, "u1")
	require.NoError(t, err)

	err = svc.StartStream(ctx, &playback.PlaySession{
		ID: "s2", ProfileID: "p1", MediaItemID: "m2",
		PlayMethod: string(playback.DirectPlay),
	}, "u1")
	require.NoError(t, err)

	sessions, err := svc.GetActiveSessions(ctx)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)

	var transcoding, direct *playback.ActiveSessionInfo
	for _, s := range sessions {
		if s.SessionID == "s1" {
			transcoding = s
		} else {
			direct = s
		}
	}
	require.NotNil(t, transcoding)
	require.NotNil(t, direct)
	assert.NotNil(t, transcoding.TranscodeInfo)
	assert.Nil(t, direct.TranscodeInfo)
}
