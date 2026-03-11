package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/gravity-zero/evey/internal/domain/playback"
	pbfakes "github.com/gravity-zero/evey/internal/domain/playback/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestPlaybackHandler() (*playbackHandler, *pbfakes.FakePlaySessionRepository, *pbfakes.FakeWatchProgressRepository, *libfakes.FakeMediaItemQueryRepo) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	eventBus := sharedfakes.NewFakeEventBus()

	userRepo.Create(context.Background(), &user.User{
		ID:       "user-1",
		Username: "admin",
		IsAdmin:  true,
		Enabled:  true,
	})
	profileRepo.Create(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Default",
	})

	cmdSvc := appplayback.NewCommandService(progressRepo, sessionRepo, sessionRepo, userRepo, profileRepo, eventBus)
	querySvc := appplayback.NewQueryService(progressRepo, sessionRepo)

	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	libQuerySvc := applibrary.NewQueryService(libQueryRepo, mediaQueryRepo, streamRepo, searchIdx)

	h := &playbackHandler{
		cmd:        cmdSvc,
		query:      querySvc,
		librarySvc: libQuerySvc,
	}
	return h, sessionRepo, progressRepo, mediaQueryRepo
}

func pbAdminCtx() context.Context {
	return interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    "user-1",
		Username:  "admin",
		IsAdmin:   true,
		ProfileID: "profile-1",
	})
}

func TestPlaybackHandler_StartStream(t *testing.T) {
	h, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	resp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DeviceName:  "Firefox",
		ClientName:  "Web",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, "profile-1", resp.ProfileId)
	assert.Equal(t, "mi-1", resp.MediaItemId)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, resp.PlayMethod)
}

func TestPlaybackHandler_StartStream_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestPlaybackHandler()

	_, err := h.StartStream(context.Background(), &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPlaybackHandler_StopStream(t *testing.T) {
	h, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	startResp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DeviceName:  "Firefox",
	})
	require.NoError(t, err)

	_, err = h.StopStream(ctx, &eveyv1.StopStreamRequest{SessionId: startResp.Id})
	require.NoError(t, err)
}

func TestPlaybackHandler_ReportProgress(t *testing.T) {
	h, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	startResp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DeviceName:  "Firefox",
	})
	require.NoError(t, err)

	_, err = h.ReportProgress(ctx, &eveyv1.ReportProgressRequest{
		SessionId:  startResp.Id,
		PositionMs: 60000,
	})
	require.NoError(t, err)

	wp, err := h.GetWatchProgress(ctx, &eveyv1.GetWatchProgressRequest{
		MediaItemId: "mi-1",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(60000), wp.PositionMs)
}

func TestPlaybackHandler_GetWatchProgress_Exists(t *testing.T) {
	h, _, progressRepo, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID:           "wp-1",
		ProfileID:    "profile-1",
		MediaItemID:  "mi-1",
		PositionMs:   45000,
		Completed:    false,
		LastPlayedAt: time.Now(),
	})

	wp, err := h.GetWatchProgress(ctx, &eveyv1.GetWatchProgressRequest{
		MediaItemId: "mi-1",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(45000), wp.PositionMs)
	assert.Equal(t, "mi-1", wp.MediaItemId)
	assert.False(t, wp.Played)
}

func TestPlaybackHandler_GetResumeItems(t *testing.T) {
	h, _, progressRepo, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID:           "wp-1",
		ProfileID:    "profile-1",
		MediaItemID:  "mi-1",
		PositionMs:   30000,
		Completed:    false,
		LastPlayedAt: time.Now(),
	})

	resp, err := h.GetResumeItems(ctx, &eveyv1.GetResumeItemsRequest{Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Progress, 1)
	assert.Equal(t, "mi-1", resp.Progress[0].MediaItemId)
	assert.Equal(t, int64(30000), resp.Progress[0].PositionMs)
}

func TestPlaybackHandler_MarkAsWatched(t *testing.T) {
	h, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	_, err := h.MarkAsWatched(ctx, &eveyv1.MarkAsWatchedRequest{
		MediaItemId: "mi-1",
	})
	require.NoError(t, err)

	wp, err := h.GetWatchProgress(ctx, &eveyv1.GetWatchProgressRequest{
		MediaItemId: "mi-1",
	})
	require.NoError(t, err)
	assert.True(t, wp.Played)
}

func TestPlaybackHandler_MarkAsUnwatched(t *testing.T) {
	h, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	_, err := h.MarkAsWatched(ctx, &eveyv1.MarkAsWatchedRequest{MediaItemId: "mi-1"})
	require.NoError(t, err)

	_, err = h.MarkAsUnwatched(ctx, &eveyv1.MarkAsUnwatchedRequest{MediaItemId: "mi-1"})
	require.NoError(t, err)

	wp, err := h.GetWatchProgress(ctx, &eveyv1.GetWatchProgressRequest{MediaItemId: "mi-1"})
	require.NoError(t, err)
	assert.False(t, wp.Played)
}

func TestPlaybackHandler_GetStreamInfo(t *testing.T) {
	h, _, _, mediaQuery := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:              "mi-1",
		Title:           "Test Movie",
		Type:            "movie",
		ContainerFormat: "mkv",
	})

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-1"})

	require.NoError(t, err)
	assert.Equal(t, "mi-1", resp.MediaItemId)
	assert.Equal(t, "mkv", resp.Container)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, resp.PlayMethod)
	assert.Contains(t, resp.DirectPlayUrl, "mi-1")
}
