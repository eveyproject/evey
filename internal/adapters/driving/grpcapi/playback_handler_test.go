package grpcapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/playback"
	pbfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mustPlaybackDurationMs builds a shared.Duration for tests; failing fast
// on a negative literal keeps fixtures terse without per-call boilerplate.
func mustPlaybackDurationMs(t *testing.T, ms int64) shared.Duration {
	t.Helper()
	d, err := shared.NewDurationMs(ms)
	if err != nil {
		t.Fatalf("NewDurationMs(%d): %v", ms, err)
	}
	return d
}

func newTestPlaybackHandler() (*playbackHandler, *pbfakes.FakePlaySessionRepository, *pbfakes.FakeWatchProgressRepository, *libfakes.FakeMediaItemQueryRepo, *userfakes.FakeProfileRepository, *libfakes.FakeMediaStreamRepository) {
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

	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	userQuery := appuser.NewQueryService(userRepo, profileRepo, apiKeyRepo, inviteRepo, qcRepo)

	h := &playbackHandler{
		cmd:        cmdSvc,
		query:      querySvc,
		librarySvc: libQuerySvc,
		userQuery:  userQuery,
	}
	return h, sessionRepo, progressRepo, mediaQueryRepo, profileRepo, streamRepo
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
	h, _, _, _, _, _ := newTestPlaybackHandler()
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
	h, _, _, _, _, _ := newTestPlaybackHandler()

	_, err := h.StartStream(context.Background(), &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPlaybackHandler_StopStream(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
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
	h, _, _, _, _, _ := newTestPlaybackHandler()
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
	h, _, progressRepo, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID:           "wp-1",
		ProfileID:    "profile-1",
		MediaItemID:  "mi-1",
		Position:     mustPlaybackDurationMs(t, 45000),
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
	h, _, progressRepo, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID:           "wp-1",
		ProfileID:    "profile-1",
		MediaItemID:  "mi-1",
		Position:     mustPlaybackDurationMs(t, 30000),
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
	h, _, _, _, _, _ := newTestPlaybackHandler()
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
	h, _, _, _, _, _ := newTestPlaybackHandler()
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
	h, _, _, mediaQuery, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-1",
		Title: "Test Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mp4"},
	})

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-1"})

	require.NoError(t, err)
	assert.Equal(t, "mi-1", resp.MediaItemId)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, resp.PlayMethod)
	assert.Contains(t, resp.DirectPlayUrl, "mi-1")
}

func TestPlaybackHandler_StartStream_ParentalBlocked(t *testing.T) {
	h, _, _, mediaQuery, profileRepo, _ := newTestPlaybackHandler()

	profileRepo.Update(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Kid",
		IsKids: true,
		Parental: &user.ParentalSettings{
			MaxAge:  intPtr(10),
			Enabled: true,
		},
	})
	mediaQuery.Seed(&library.MediaItem{
		ID:             "mi-r",
		Title:          "R-Rated Movie",
		Type:           "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"},
	})

	ctx := pbAdminCtx()
	_, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-r",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "parental_blocked")
}

func TestPlaybackHandler_StartStream_ParentalAllowed(t *testing.T) {
	h, _, _, mediaQuery, profileRepo, _ := newTestPlaybackHandler()

	profileRepo.Update(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Kid",
		IsKids: true,
		Parental: &user.ParentalSettings{
			MaxAge:  intPtr(13),
			Enabled: true,
		},
	})
	mediaQuery.Seed(&library.MediaItem{
		ID:             "mi-pg",
		Title:          "PG Movie",
		Type:           "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"},
	})

	ctx := pbAdminCtx()
	resp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-pg",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
}

func TestPlaybackHandler_StartStream_ParentalDisabled(t *testing.T) {
	h, _, _, mediaQuery, profileRepo, _ := newTestPlaybackHandler()

	profileRepo.Update(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Default",
		Parental: &user.ParentalSettings{
			MaxAge:  intPtr(10),
			Enabled:   false,
		},
	})
	mediaQuery.Seed(&library.MediaItem{
		ID:             "mi-r2",
		Title:          "R-Rated Movie 2",
		Type:           "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"},
	})

	ctx := pbAdminCtx()
	resp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-r2",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
}

func TestPlaybackHandler_StartStream_ParentalBlockedTag(t *testing.T) {
	h, _, _, mediaQuery, profileRepo, _ := newTestPlaybackHandler()

	profileRepo.Update(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Kid",
		IsKids: true,
		Parental: &user.ParentalSettings{
			BlockedTags: []string{"violence"},
			Enabled:     true,
		},
	})
	mediaQuery.Seed(&library.MediaItem{
		ID:      "mi-v",
		Title:   "Violent Movie",
		Type:    "movie",
		Catalog: &library.MediaCatalog{Tags: []string{"Violence", "Action"}},
	})

	ctx := pbAdminCtx()
	_, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-v",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "parental_blocked")
}

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }

// newTestPlaybackHandlerWithAccess builds a playbackHandler that has a real
// userAccess service wired in (unlike newTestPlaybackHandler which leaves it
// nil). This makes requireMediaAccess exercise the actual library-access check
// instead of short-circuiting via the "userAccess == nil" guard.
func newTestPlaybackHandlerWithAccess() (*playbackHandler, *libfakes.FakeMediaItemQueryRepo, *libfakes.FakeMediaStreamRepository, *userfakes.FakeLibraryAccessRepository) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	eventBus := sharedfakes.NewFakeEventBus()

	userRepo.Create(context.Background(), &user.User{
		ID:       "user-2",
		Username: "viewer",
		IsAdmin:  false,
		Enabled:  true,
	})
	profileRepo.Create(context.Background(), &user.Profile{
		ID:     "profile-2",
		UserID: "user-2",
		Name:   "Default",
	})

	cmdSvc := appplayback.NewCommandService(progressRepo, sessionRepo, sessionRepo, userRepo, profileRepo, eventBus)
	querySvc := appplayback.NewQueryService(progressRepo, sessionRepo)

	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	libQuerySvc := applibrary.NewQueryService(libQueryRepo, mediaQueryRepo, streamRepo, searchIdx)

	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	libraryAccess := userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userRepo, apiKeyRepo, inviteRepo, qcRepo,
		userfakes.NewFakePasswordHasher(), eventBus,
		appuser.WithLibraryAccess(libraryAccess),
	)
	userQuery := appuser.NewQueryService(userRepo, profileRepo, apiKeyRepo, inviteRepo, qcRepo)

	h := &playbackHandler{
		cmd:        cmdSvc,
		query:      querySvc,
		librarySvc: libQuerySvc,
		userQuery:  userQuery,
		userAccess: userAccess,
	}
	return h, mediaQueryRepo, streamRepo, libraryAccess
}

func TestGetStreamInfo_AudioCodecsWired(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-audio",
		Title: "DTS Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mp4"},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-audio", StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{MediaItemID: "mi-audio", StreamType: "audio", Codec: "dts", Channels: intPtr(6), IsDefault: true},
	})

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId:          "mi-audio",
		SupportedAudioCodecs: []string{"aac", "mp3"},
	})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE, resp.PlayMethod)
	assert.Contains(t, resp.TranscodeReasons, "audio_codec_unsupported")
	assert.Equal(t, "transcode", resp.AudioDecision)
	assert.Equal(t, "copy", resp.VideoDecision)
}

func TestGetStreamInfo_AudioCodecsDirectPlay(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-dp",
		Title: "AAC Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mp4"},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-dp", StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{MediaItemID: "mi-dp", StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	})

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId:          "mi-dp",
		SupportedAudioCodecs: []string{"aac", "opus"},
	})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, resp.PlayMethod)
	assert.Empty(t, resp.TranscodeReasons)
	assert.Equal(t, "copy", resp.AudioDecision)
	assert.Equal(t, "copy", resp.VideoDecision)
}

func TestPlaybackHandler_GetWatchHistory(t *testing.T) {
	h, _, progressRepo, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-1", ProfileID: "profile-1", MediaItemID: "mi-1",
		Position: mustPlaybackDurationMs(t, 30000), LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-2", ProfileID: "profile-1", MediaItemID: "mi-2",
		Position: mustPlaybackDurationMs(t, 60000), LastPlayedAt: time.Now(),
	})

	resp, err := h.GetWatchHistory(ctx, &eveyv1.GetWatchHistoryRequest{Limit: 10})

	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

func TestPlaybackHandler_GetWatchHistory_EnrichesEpisodeLabel(t *testing.T) {
	h, _, progressRepo, mediaRepo, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	// episode -> season -> series hierarchy; season/episode numbers via IndexNumber.
	seasonNum, epNum := 2, 5
	mediaRepo.Seed(&library.MediaItem{ID: "series-1", Type: library.Series, Title: "Breaking Bad"})
	mediaRepo.Seed(&library.MediaItem{ID: "season-1", Type: library.Season, ParentID: "series-1", IndexNumber: &seasonNum})
	mediaRepo.Seed(&library.MediaItem{ID: "ep-1", Type: library.Episode, ParentID: "season-1", Title: "Cancer Man", IndexNumber: &epNum})
	mediaRepo.Seed(&library.MediaItem{ID: "movie-1", Type: library.MovieItem, Title: "Heat"})

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{ID: "wp-ep", ProfileID: "profile-1", MediaItemID: "ep-1", LastPlayedAt: time.Now()})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{ID: "wp-mov", ProfileID: "profile-1", MediaItemID: "movie-1", LastPlayedAt: time.Now().Add(-time.Minute)})

	resp, err := h.GetWatchHistory(ctx, &eveyv1.GetWatchHistoryRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)

	byID := map[string]*eveyv1.WatchProgress{}
	for _, it := range resp.Items {
		byID[it.MediaItemId] = it
	}
	ep := byID["ep-1"]
	require.NotNil(t, ep)
	assert.Equal(t, "Cancer Man", ep.Title)
	assert.Equal(t, "episode", ep.MediaType)
	assert.Equal(t, int32(5), ep.IndexNumber)
	assert.Equal(t, int32(2), ep.ParentIndexNumber)
	assert.Equal(t, "Breaking Bad", ep.SeriesTitle)

	mov := byID["movie-1"]
	require.NotNil(t, mov)
	assert.Equal(t, "Heat", mov.Title)
	assert.Equal(t, "movie", mov.MediaType)
	assert.Empty(t, mov.SeriesTitle)
}

func TestPlaybackHandler_GetWatchHistory_Unauthenticated(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
	_, err := h.GetWatchHistory(context.Background(), &eveyv1.GetWatchHistoryRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPlaybackHandler_GetWatchHistory_Pagination(t *testing.T) {
	h, _, progressRepo, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	for i := range 5 {
		progressRepo.Upsert(context.Background(), &playback.WatchProgress{
			ID: fmt.Sprintf("wp-%d", i), ProfileID: shared.ProfileID("profile-1"),
			MediaItemID: shared.MediaItemID(fmt.Sprintf("mi-%d", i)), Position: mustPlaybackDurationMs(t, int64(i*1000)),
			LastPlayedAt: time.Now(),
		})
	}

	resp, err := h.GetWatchHistory(ctx, &eveyv1.GetWatchHistoryRequest{
		Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 2},
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

func TestPlaybackHandler_GetNextUp(t *testing.T) {
	h, _, progressRepo, mediaQuery, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-1", ProfileID: "profile-1", MediaItemID: "mi-ep1",
		Position: mustPlaybackDurationMs(t, 30000), Completed: true, LastPlayedAt: time.Now(),
	})
	mediaQuery.Seed(&library.MediaItem{
		ID: "mi-ep1", Title: "Episode 1", Type: "episode",
	})

	resp, err := h.GetNextUp(ctx, &eveyv1.GetNextUpRequest{Limit: 10})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestPlaybackHandler_GetNextUp_Unauthenticated(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
	_, err := h.GetNextUp(context.Background(), &eveyv1.GetNextUpRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPlaybackHandler_MarkAsWatched_Unauthenticated(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
	_, err := h.MarkAsWatched(context.Background(), &eveyv1.MarkAsWatchedRequest{MediaItemId: "mi-1"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPlaybackHandler_MarkAsUnwatched_Unauthenticated(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
	_, err := h.MarkAsUnwatched(context.Background(), &eveyv1.MarkAsUnwatchedRequest{MediaItemId: "mi-1"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGetStreamInfo_DecisionFieldsPopulated(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-hevc",
		Title: "HEVC Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "matroska"},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-hevc", StreamType: "video", Codec: "hevc", Width: intPtr(3840), Height: intPtr(2160), Bitrate: int64Ptr(20000000), IsDefault: true},
		{MediaItemID: "mi-hevc", StreamType: "audio", Codec: "dts", Channels: intPtr(6), IsDefault: true},
	})

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId: "mi-hevc",
	})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE, resp.PlayMethod)
	assert.Equal(t, "transcode", resp.VideoDecision)
	assert.Equal(t, "transcode", resp.AudioDecision)
	assert.Contains(t, resp.TranscodeReasons, "video_codec_unsupported")
	assert.Contains(t, resp.TranscodeReasons, "audio_codec_unsupported")
	assert.Contains(t, resp.TranscodeReasons, "container_incompatible")
}

// TestPlaybackHandler_GetStreamInfo_CrossLibrary_NotFound verifies that a
// non-admin user requesting stream info for a media item whose library is NOT
// in their access list receives codes.NotFound (IDOR guard). Without this
// test the short-circuit "h.userAccess == nil → allow" in requireMediaAccess
// would mask any regression that made the check permissive.
func TestPlaybackHandler_GetStreamInfo_CrossLibrary_NotFound(t *testing.T) {
	h, mediaQueryRepo, _, libraryAccess := newTestPlaybackHandlerWithAccess()

	// Grant user-2 access to lib-1 only; lib-secret is not in the list.
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	// Seed a media item that lives in lib-secret (the forbidden library).
	mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-secret",
		LibraryID: "lib-secret",
		Title:     "Secret Movie",
		Type:      "movie",
		File:      &library.MediaFile{ContainerFormat: "mp4"},
	})

	ctx := userCtx("user-2", "profile-2")
	_, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-secret"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err), "cross-library access must be denied with NotFound")
}

// TestPlaybackHandler_GetStreamInfo_AllowedLibrary verifies the positive path:
// the same user CAN access a media item whose library IS in their grant list.
// This ensures the test above actually discriminates (a permissive check would
// make both pass, not just the denial test).
func TestPlaybackHandler_GetStreamInfo_AllowedLibrary(t *testing.T) {
	h, mediaQueryRepo, _, libraryAccess := newTestPlaybackHandlerWithAccess()

	// Grant user-2 access to lib-1.
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	// Seed a media item in the allowed library.
	mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-allowed",
		LibraryID: "lib-1",
		Title:     "Allowed Movie",
		Type:      "movie",
		File:      &library.MediaFile{ContainerFormat: "mp4"},
	})

	ctx := userCtx("user-2", "profile-2")
	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-allowed"})

	require.NoError(t, err)
	assert.Equal(t, "mi-allowed", resp.MediaItemId)
}

// TestPlaybackHandler_StartStream_CrossLibrary_NotFound verifies that
// StartStream also enforces library access (it calls requireMediaAccess at
// l.181 of playback_handler.go).
func TestPlaybackHandler_StartStream_CrossLibrary_NotFound(t *testing.T) {
	h, mediaQueryRepo, _, libraryAccess := newTestPlaybackHandlerWithAccess()

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	mediaQueryRepo.Seed(&library.MediaItem{
		ID:        "mi-secret2",
		LibraryID: "lib-secret",
		Title:     "Secret Movie 2",
		Type:      "movie",
	})

	ctx := userCtx("user-2", "profile-2")
	_, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-secret2",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err), "cross-library StartStream must be denied with NotFound")
}
