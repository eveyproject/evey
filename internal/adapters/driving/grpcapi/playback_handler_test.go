package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/playback"
	pbfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/transcoding"
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
		userAccess: newPermissiveTestUserAccess(),
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

// pbUserCtx is a non-admin caller bound to profileID - used to prove the
// per-session ownership guards (a regular user cannot touch another profile's
// session). Non-admin is required: the guards are bypassed for admins.
func pbUserCtx(userID, profileID string) context.Context {
	return interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    userID,
		Username:  "viewer",
		IsAdmin:   false,
		ProfileID: profileID,
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

func TestPlaybackHandler_StartStream_StampsServedBitrate(t *testing.T) {
	h, sessionRepo, _, mediaQuery, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()
	mediaQuery.Seed(&library.MediaItem{ID: "mi-srv", Title: "X", Type: "movie", File: &library.MediaFile{Bitrate: 20_000_000}})

	resp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId:      "mi-srv",
		PlayMethod:       eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE,
		DeviceName:       "Firefox",
		ServedBitrateBps: 4_192_000,
	})
	require.NoError(t, err)
	stored, err := sessionRepo.GetByID(context.Background(), resp.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(4_192_000), stored.BandwidthBps, "the client-reported served bitrate is stamped, not the 20 Mbps source")
}

func TestPlaybackHandler_StartStream_FallsBackToSourceBitrate(t *testing.T) {
	h, sessionRepo, _, mediaQuery, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()
	mediaQuery.Seed(&library.MediaItem{ID: "mi-src", Title: "X", Type: "movie", File: &library.MediaFile{Bitrate: 8_000_000}})

	resp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-src",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DeviceName:  "Firefox",
		// no ServedBitrateBps → older client → fall back to source bitrate
	})
	require.NoError(t, err)
	stored, err := sessionRepo.GetByID(context.Background(), resp.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(8_000_000), stored.BandwidthBps, "absent served bitrate falls back to source")
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

// TestPlaybackHandler_StopStream_KeepsTranscodeWhileOtherTabActive locks in the
// multi-tab fix: closing ONE of two tabs on the same film must not tear down the
// shared transcode the other tab is still consuming. lastActiveStreamForMedia
// gates the (forceful) RemoveSession so it only fires when this was the LAST
// active stream for the media.
func TestPlaybackHandler_StopStream_KeepsTranscodeWhileOtherTabActive(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	// Two tabs on the SAME media → two active sessions.
	s1, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{MediaItemId: "mi-1", DeviceName: "tab1"})
	require.NoError(t, err)
	s2, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{MediaItemId: "mi-1", DeviceName: "tab2"})
	require.NoError(t, err)

	// Closing tab2 while tab1 is still active is NOT the last stream → keep the transcode.
	assert.False(t, h.lastActiveStreamForMedia(ctx, "mi-1", s2.Id),
		"another tab is still streaming mi-1 - the shared transcode must NOT be removed")

	// Stop tab2 (marks it ended); only tab1 remains active.
	_, err = h.StopStream(ctx, &eveyv1.StopStreamRequest{SessionId: s2.Id})
	require.NoError(t, err)

	// Closing tab1 now IS the last stream → the transcode may be torn down.
	assert.True(t, h.lastActiveStreamForMedia(ctx, "mi-1", s1.Id),
		"the last remaining tab closing IS the last stream - the transcode may be removed")
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

// TestPlaybackHandler_StopStream_IDOR_OtherProfileDenied locks in the ownership
// guard: a non-admin profile must not be able to stop another profile's session.
func TestPlaybackHandler_StopStream_IDOR_OtherProfileDenied(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()

	// profile-1 owns the session.
	startResp, err := h.StartStream(pbAdminCtx(), &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DeviceName:  "Firefox",
	})
	require.NoError(t, err)

	// profile-2 (a different, non-admin user) tries to stop it → PermissionDenied.
	_, err = h.StopStream(pbUserCtx("user-2", "profile-2"), &eveyv1.StopStreamRequest{SessionId: startResp.Id})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	// The session must still be stoppable by its owner afterwards.
	_, err = h.StopStream(pbAdminCtx(), &eveyv1.StopStreamRequest{SessionId: startResp.Id})
	require.NoError(t, err)
}

// TestPlaybackHandler_ReportProgress_IDOR_OtherProfileDenied locks in the
// ownership guard on progress reporting (a non-admin cannot write progress onto
// another profile's session).
func TestPlaybackHandler_ReportProgress_IDOR_OtherProfileDenied(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()

	startResp, err := h.StartStream(pbAdminCtx(), &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DeviceName:  "Firefox",
	})
	require.NoError(t, err)

	_, err = h.ReportProgress(pbUserCtx("user-2", "profile-2"), &eveyv1.ReportProgressRequest{
		SessionId:  startResp.Id,
		PositionMs: 60000,
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
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

// GetStreamInfo must hide a parental-blocked item exactly like StartStream:
// returning stream metadata/URLs (or anything other than NotFound) would let a
// kids profile harvest an adult item's existence and codecs by media ID.
func TestPlaybackHandler_GetStreamInfo_ParentalBlocked_NotFound(t *testing.T) {
	h, _, _, mediaQuery, profileRepo, _ := newTestPlaybackHandler()

	profileRepo.Update(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Kid",
		IsKids: true,
		Parental: &user.ParentalSettings{
			MaxAge:  new(10),
			Enabled: true,
		},
	})
	mediaQuery.Seed(&library.MediaItem{
		ID:             "mi-r",
		Title:          "R-Rated Movie",
		Type:           "movie",
		File:           &library.MediaFile{ContainerFormat: "mp4"},
		Classification: &library.MediaClassification{OfficialRating: "R"},
	})

	// Non-admin on purpose: an admin is exempt from the parental gate
	// (resolveParentalFilterFromUserQuery), so an admin ctx here would assert
	// the opposite of the intended rule. The rule under test is "a KIDS
	// profile is blocked".
	ctx := pbUserCtx("user-1", "profile-1")
	_, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-r"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
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

	// Non-admin on purpose: an admin is exempt from the parental gate
	// (resolveParentalFilterFromUserQuery), so an admin ctx here would assert
	// the opposite of the intended rule. The rule under test is "a KIDS
	// profile is blocked".
	ctx := pbUserCtx("user-1", "profile-1")
	_, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-r",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.Error(t, err)
	// Parental block returns NotFound (hide existence), not PermissionDenied.
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "not_found")
}

func TestPlaybackHandler_StartStream_ParentalBlocked_ForeignCertOnly(t *testing.T) {
	h, _, _, mediaQuery, profileRepo, _ := newTestPlaybackHandler()

	profileRepo.Update(context.Background(), &user.Profile{
		ID:     "profile-1",
		UserID: "user-1",
		Name:   "Kid",
		Region: new("FR"),
		IsKids: true,
		Parental: &user.ParentalSettings{
			MaxAge:  new(10),
			Enabled: true,
		},
	})
	// Adult rating present only as a foreign cert (US: R), no FR cert and no
	// OfficialRating. The old single-country permissive check let this through at
	// StartStream while browse blocked it (worst-case). Must now be blocked too.
	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-foreign",
		Title: "Foreign-cert Adult Movie",
		Type:  "movie",
		Classification: &library.MediaClassification{
			CertificationsByCountry: map[string]string{"US": "R"},
		},
	})

	// Non-admin on purpose: an admin is exempt from the parental gate
	// (resolveParentalFilterFromUserQuery), so an admin ctx here would assert
	// the opposite of the intended rule. The rule under test is "a KIDS
	// profile is blocked".
	ctx := pbUserCtx("user-1", "profile-1")
	_, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-foreign",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.Error(t, err)
	// Parental block returns NotFound (hide existence), not PermissionDenied.
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "not_found")
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
			Enabled: false,
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

	// Non-admin on purpose: an admin is exempt from the parental gate
	// (resolveParentalFilterFromUserQuery), so an admin ctx here would assert
	// the opposite of the intended rule. The rule under test is "a KIDS
	// profile is blocked".
	ctx := pbUserCtx("user-1", "profile-1")
	_, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-v",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	})

	require.Error(t, err)
	// Parental block returns NotFound (hide existence), not PermissionDenied.
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "not_found")
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

// A copy-eligible video/audio pair in a browser-incompatible container (MKV)
// must resolve to direct_stream and be routed to the HLS master so the copy
// rung can repackage it - NOT to the raw /stream byte-server, which would hand
// the client an undecodable MKV. Regression guard for the formerly no-op
// direct_stream branch (it returned DirectPlayUrl=/stream).
func TestGetStreamInfo_DirectStream_RoutesToHLS(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-ds",
		Title: "Remux Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mkv"},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-ds", StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{MediaItemID: "mi-ds", StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	})

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-ds"})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM, resp.PlayMethod)
	assert.Equal(t, "copy", resp.VideoDecision)
	assert.Equal(t, "copy", resp.AudioDecision)
	// The fix: HLS master, not the raw byte-server. DirectPlayUrl must be empty
	// so no client falls back to serving the undecodable original container.
	assert.Contains(t, resp.TranscodeUrl, "mi-ds")
	assert.Contains(t, resp.TranscodeUrl, "hls/master.m3u8")
	assert.Empty(t, resp.DirectPlayUrl)
}

// B6(b): GetStreamInfo must honor the client's pre-selected tracks (absolute
// probed indices). A selected PGS subtitle forces the burn-in transcode -
// advertising direct_play would silently drop the subtitle (bitmap subs can't
// be sidecar'd as text).
func TestGetStreamInfo_SelectedBitmapSubtitleForcesBurnIn(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-pgs",
		Title: "PGS Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mp4"},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-pgs", StreamIndex: 0, StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{MediaItemID: "mi-pgs", StreamIndex: 1, StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{MediaItemID: "mi-pgs", StreamIndex: 2, StreamType: "subtitle", Codec: "subrip"},
		{MediaItemID: "mi-pgs", StreamIndex: 3, StreamType: "subtitle", Codec: "hdmv_pgs_subtitle"},
	})

	// Without a selection: direct play (the sub doesn't block anything).
	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-pgs"})
	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, resp.PlayMethod)

	// PGS selected (absolute index 3 → subtitle-relative 1): burn-in transcode.
	resp, err = h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId:         "mi-pgs",
		SubtitleStreamIndex: 3,
	})
	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE, resp.PlayMethod)
	assert.Contains(t, resp.TranscodeReasons, "subtitle_burn_in")
	assert.Equal(t, "transcode", resp.VideoDecision)

	// Text sub selected (absolute 2): sidecar-deliverable, still direct play.
	resp, err = h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId:         "mi-pgs",
		SubtitleStreamIndex: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, resp.PlayMethod)
}

// B6(b) audio side: the decision judges the SELECTED audio track, not the
// container default - the gRPC mirror of the HLS-side fix (default DTS +
// selected AAC = copy; default AAC + selected DTS = transcode).
func TestGetStreamInfo_SelectedAudioTrackJudged(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-sel-audio",
		Title: "Dual Audio Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mp4"},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-sel-audio", StreamIndex: 0, StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{MediaItemID: "mi-sel-audio", StreamIndex: 1, StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true},
		{MediaItemID: "mi-sel-audio", StreamIndex: 2, StreamType: "audio", Codec: "aac", Channels: new(2)},
	})

	// Default (DTS) judged without a selection → audio transcode.
	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId:          "mi-sel-audio",
		SupportedAudioCodecs: []string{"aac", "mp3"},
	})
	require.NoError(t, err)
	assert.Equal(t, "transcode", resp.AudioDecision)

	// AAC track selected (absolute 2 → audio-relative 1) → audio copy.
	resp, err = h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId:          "mi-sel-audio",
		SupportedAudioCodecs: []string{"aac", "mp3"},
		AudioStreamIndex:     2,
	})
	require.NoError(t, err)
	assert.Equal(t, "copy", resp.AudioDecision)

	// Unknown index → ignored, default-track behaviour.
	resp, err = h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{
		MediaItemId:          "mi-sel-audio",
		SupportedAudioCodecs: []string{"aac", "mp3"},
		AudioStreamIndex:     99,
	})
	require.NoError(t, err)
	assert.Equal(t, "transcode", resp.AudioDecision)
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
	mediaRepo.Seed(&library.MediaItem{ID: "series-1", Type: library.Series, Title: "Some Series"})
	mediaRepo.Seed(&library.MediaItem{ID: "season-1", Type: library.Season, ParentID: "series-1", IndexNumber: &seasonNum})
	mediaRepo.Seed(&library.MediaItem{ID: "ep-1", Type: library.Episode, ParentID: "season-1", Title: "Some Episode", IndexNumber: &epNum})
	mediaRepo.Seed(&library.MediaItem{ID: "movie-1", Type: library.MovieItem, Title: "Some Movie"})

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
	assert.Equal(t, "Some Episode", ep.Title)
	assert.Equal(t, "episode", ep.MediaType)
	assert.Equal(t, int32(5), ep.IndexNumber)
	assert.Equal(t, int32(2), ep.ParentIndexNumber)
	assert.Equal(t, "Some Series", ep.SeriesTitle)

	mov := byID["movie-1"]
	require.NotNil(t, mov)
	assert.Equal(t, "Some Movie", mov.Title)
	assert.Equal(t, "movie", mov.MediaType)
	assert.Empty(t, mov.SeriesTitle)
}

// A progress row stores where the viewer stopped; how long the film is only
// the media item knows - and no handler ever filled duration_ms or
// percent_complete, so every client's history line and progress bar sat on
// zero (found on mobile 2026-08-03, true of web and TV too).
func TestPlaybackHandler_GetWatchHistory_FillsDurationAndPercent(t *testing.T) {
	h, _, progressRepo, mediaRepo, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	dur, err := shared.NewDurationMs(120_000)
	require.NoError(t, err)
	mediaRepo.Seed(&library.MediaItem{
		ID: "mi-dur", Type: library.MovieItem, Title: "Some Movie",
		File: &library.MediaFile{Path: "/f.mkv", Duration: dur},
	})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-nofile", Type: library.MovieItem, Title: "Lost Media"})
	mediaRepo.Seed(&library.MediaItem{
		ID: "mi-over", Type: library.MovieItem, Title: "Short Header",
		File: &library.MediaFile{Path: "/g.mkv", Duration: dur},
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-dur", ProfileID: "profile-1", MediaItemID: "mi-dur",
		Position: mustPlaybackDurationMs(t, 30_000), LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-nofile", ProfileID: "profile-1", MediaItemID: "mi-nofile",
		Position: mustPlaybackDurationMs(t, 10_000), LastPlayedAt: time.Now().Add(-time.Minute),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-over", ProfileID: "profile-1", MediaItemID: "mi-over",
		Position: mustPlaybackDurationMs(t, 130_000), LastPlayedAt: time.Now().Add(-2 * time.Minute),
	})

	resp, err := h.GetWatchHistory(ctx, &eveyv1.GetWatchHistoryRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 3)
	byID := map[string]*eveyv1.WatchProgress{}
	for _, it := range resp.Items {
		byID[it.MediaItemId] = it
	}
	require.NotNil(t, byID["mi-dur"])
	assert.Equal(t, int64(120_000), byID["mi-dur"].DurationMs)
	assert.InDelta(t, 25.0, byID["mi-dur"].PercentComplete, 0.01)
	// No file, no declared length: the fields stay honestly at zero.
	require.NotNil(t, byID["mi-nofile"])
	assert.Zero(t, byID["mi-nofile"].DurationMs)
	assert.Zero(t, byID["mi-nofile"].PercentComplete)
	// A position past a short-header's declared length clamps at 100, not 108.
	require.NotNil(t, byID["mi-over"])
	assert.InDelta(t, 100.0, byID["mi-over"].PercentComplete, 0.01)
}

func TestPlaybackHandler_GetResumeItems_FillsDurationAndPercent(t *testing.T) {
	h, _, progressRepo, mediaRepo, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	dur, err := shared.NewDurationMs(200_000)
	require.NoError(t, err)
	mediaRepo.Seed(&library.MediaItem{
		ID: "mi-r1", Type: library.MovieItem, Title: "Film",
		File: &library.MediaFile{Path: "/f.mkv", Duration: dur},
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-r1", ProfileID: "profile-1", MediaItemID: "mi-r1",
		Position: mustPlaybackDurationMs(t, 50_000), LastPlayedAt: time.Now(),
	})

	resp, err := h.GetResumeItems(ctx, &eveyv1.GetResumeItemsRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Progress, 1)
	assert.Equal(t, int64(200_000), resp.Progress[0].DurationMs)
	assert.InDelta(t, 25.0, resp.Progress[0].PercentComplete, 0.01)
}

func TestPlaybackHandler_GetWatchProgress_FillsDurationAndPercent(t *testing.T) {
	h, _, progressRepo, mediaRepo, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	dur, err := shared.NewDurationMs(100_000)
	require.NoError(t, err)
	mediaRepo.Seed(&library.MediaItem{
		ID: "mi-p1", Type: library.MovieItem, Title: "Film",
		File: &library.MediaFile{Path: "/f.mkv", Duration: dur},
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-p1", ProfileID: "profile-1", MediaItemID: "mi-p1",
		Position: mustPlaybackDurationMs(t, 75_000), LastPlayedAt: time.Now(),
	})

	wp, err := h.GetWatchProgress(ctx, &eveyv1.GetWatchProgressRequest{MediaItemId: "mi-p1"})
	require.NoError(t, err)
	assert.Equal(t, int64(100_000), wp.DurationMs)
	assert.InDelta(t, 75.0, wp.PercentComplete, 0.01)
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

func TestPlaybackHandler_GetNextUp_ParentalFiltersAdult(t *testing.T) {
	h, _, progressRepo, mediaQuery, profileRepo, _ := newTestPlaybackHandler()
	ctx := context.Background()

	maxAge := 13
	profileRepo.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})

	progressRepo.Upsert(ctx, &playback.WatchProgress{ID: "wp-r", ProfileID: "profile-2", MediaItemID: "mi-r",
		Position: mustPlaybackDurationMs(t, 30000), Completed: true, LastPlayedAt: time.Now()})
	progressRepo.Upsert(ctx, &playback.WatchProgress{ID: "wp-pg", ProfileID: "profile-2", MediaItemID: "mi-pg",
		Position: mustPlaybackDurationMs(t, 30000), Completed: true, LastPlayedAt: time.Now()})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-r", Title: "Adult", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}}) // age 17 → blocked
	mediaQuery.Seed(&library.MediaItem{ID: "mi-pg", Title: "Family", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}}) // age 10 → allowed

	resp, err := h.GetNextUp(userCtx("user-2", "profile-2"), &eveyv1.GetNextUpRequest{Limit: 10})
	require.NoError(t, err)
	ids := make([]string, len(resp.Items))
	for i, it := range resp.Items {
		ids[i] = it.Id
	}
	assert.NotContains(t, ids, "mi-r", "adult next-up item must be hidden from a kids profile (metadata leak)")
	assert.Contains(t, ids, "mi-pg", "kid-safe next-up item must remain")
}

func TestPlaybackHandler_GetResumeItems_ParentalFiltersAdult(t *testing.T) {
	h, _, progressRepo, mediaQuery, profileRepo, _ := newTestPlaybackHandler()
	ctx := context.Background()

	maxAge := 13
	profileRepo.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})

	progressRepo.Upsert(ctx, &playback.WatchProgress{ID: "wp-r", ProfileID: "profile-2", MediaItemID: "mi-r",
		Position: mustPlaybackDurationMs(t, 30000), Completed: false, LastPlayedAt: time.Now()})
	progressRepo.Upsert(ctx, &playback.WatchProgress{ID: "wp-pg", ProfileID: "profile-2", MediaItemID: "mi-pg",
		Position: mustPlaybackDurationMs(t, 30000), Completed: false, LastPlayedAt: time.Now()})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-r", Title: "Adult", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-pg", Title: "Family", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}})

	resp, err := h.GetResumeItems(userCtx("user-2", "profile-2"), &eveyv1.GetResumeItemsRequest{Limit: 10})
	require.NoError(t, err)
	ids := make([]string, len(resp.Progress))
	for i, p := range resp.Progress {
		ids[i] = p.MediaItemId
	}
	assert.NotContains(t, ids, "mi-r", "adult resume entry must be hidden from a kids profile")
	assert.Contains(t, ids, "mi-pg", "kid-safe resume entry must remain")
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

// TestGetStreamInfo_ServedBitrate pins resp.Bitrate to the bitrate actually
// delivered: the source rate for direct play, the capped target for a
// transcode (far below a fat source) - the value the player and dashboard show.
func TestGetStreamInfo_ServedBitrate(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID: "mi-srv-dp", Title: "DP", Type: "movie",
		File: &library.MediaFile{ContainerFormat: "mp4", Bitrate: 5_000_000},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-srv-dp", StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{MediaItemID: "mi-srv-dp", StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	})
	dp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-srv-dp", SupportedAudioCodecs: []string{"aac"}})
	require.NoError(t, err)
	require.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, dp.PlayMethod)
	assert.Equal(t, int64(5_000_000), dp.Bitrate, "direct play serves the source bitrate")

	mediaQuery.Seed(&library.MediaItem{
		ID: "mi-srv-tc", Title: "TC", Type: "movie",
		File: &library.MediaFile{ContainerFormat: "matroska", Bitrate: 20_000_000},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-srv-tc", StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160), Bitrate: int64Ptr(20000000), IsDefault: true},
		{MediaItemID: "mi-srv-tc", StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true},
	})
	tc, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-srv-tc"})
	require.NoError(t, err)
	require.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE, tc.PlayMethod)
	// The ladder follows the source bitrate (no cap set): the panel quotes the
	// source, not a placeholder.
	assert.Equal(t, int64(20_000_000), tc.Bitrate, "a transcode with no bitrate cap follows the source")
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

// newTestPlaybackHandlerFull builds a playbackHandler with a wired userAccess
// service (so the library-access filter inside GetWatchHistory, GetResumeItems,
// and GetNextUp is exercised) and also exposes the watch-progress repository so
// tests can seed progress rows. It registers user-1 (admin, profile-1) and
// user-2 (non-admin, profile-2) so a single handler covers both the filtered
// and the admin-bypass code paths.
func newTestPlaybackHandlerFull() (
	*playbackHandler,
	*pbfakes.FakeWatchProgressRepository,
	*libfakes.FakeMediaItemQueryRepo,
	*userfakes.FakeLibraryAccessRepository,
) {
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	eventBus := sharedfakes.NewFakeEventBus()

	userRepo.Create(context.Background(), &user.User{ID: "user-1", Username: "admin", IsAdmin: true, Enabled: true})
	profileRepo.Create(context.Background(), &user.Profile{ID: "profile-1", UserID: "user-1", Name: "Admin"})
	userRepo.Create(context.Background(), &user.User{ID: "user-2", Username: "viewer", IsAdmin: false, Enabled: true})
	profileRepo.Create(context.Background(), &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Default"})

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
	return h, progressRepo, mediaQueryRepo, libraryAccess
}

// TestPlaybackHandler_GetWatchHistory_LibraryAccessFilter proves that
// enrichWatchHistory drops items whose library is not in the caller's grant
// list. Without userAccess wired (as in newTestPlaybackHandler, which leaves it
// nil), getAllowedLibraryIDs always returns nil and the filter is never entered,
// so a regression leaking forbidden-library titles would pass green.
func TestPlaybackHandler_GetWatchHistory_LibraryAccessFilter(t *testing.T) {
	h, progressRepo, mediaQueryRepo, libraryAccess := newTestPlaybackHandlerFull()

	// Two items: one in the allowed library, one the caller is not granted.
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-hist-ok", LibraryID: "lib-allowed", Type: "movie", Title: "Allowed"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-hist-no", LibraryID: "lib-forbidden", Type: "movie", Title: "Forbidden"})

	// user-2 may only access lib-allowed.
	libraryAccess.Grant(context.Background(), "user-2", "lib-allowed")

	// Seed watch history for the restricted user (profile-2).
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-hist-ok", ProfileID: "profile-2", MediaItemID: "mi-hist-ok",
		Position: mustPlaybackDurationMs(t, 10000), LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-hist-no", ProfileID: "profile-2", MediaItemID: "mi-hist-no",
		Position: mustPlaybackDurationMs(t, 10000), LastPlayedAt: time.Now(),
	})

	// Seed watch history for the admin (profile-1) so the bypass sub-test can
	// assert that both items are visible without a filter.
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-hist-adm-ok", ProfileID: "profile-1", MediaItemID: "mi-hist-ok",
		Position: mustPlaybackDurationMs(t, 10000), LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-hist-adm-no", ProfileID: "profile-1", MediaItemID: "mi-hist-no",
		Position: mustPlaybackDurationMs(t, 10000), LastPlayedAt: time.Now(),
	})

	t.Run("restricted user sees only allowed-library items", func(t *testing.T) {
		resp, err := h.GetWatchHistory(pbUserCtx("user-2", "profile-2"), &eveyv1.GetWatchHistoryRequest{Limit: 10})
		require.NoError(t, err)
		ids := make([]string, len(resp.Items))
		for i, it := range resp.Items {
			ids[i] = it.MediaItemId
		}
		assert.Contains(t, ids, "mi-hist-ok", "allowed-library item must appear in history")
		assert.NotContains(t, ids, "mi-hist-no", "forbidden-library item must be filtered out of history")
	})

	t.Run("admin ctx bypasses library filter and sees both", func(t *testing.T) {
		resp, err := h.GetWatchHistory(pbAdminCtx(), &eveyv1.GetWatchHistoryRequest{Limit: 10})
		require.NoError(t, err)
		ids := make([]string, len(resp.Items))
		for i, it := range resp.Items {
			ids[i] = it.MediaItemId
		}
		assert.Contains(t, ids, "mi-hist-ok", "admin must see the allowed-library item")
		assert.Contains(t, ids, "mi-hist-no", "admin must see the forbidden-library item (no filter for admins)")
	})
}

// TestPlaybackHandler_GetResumeItems_LibraryAccessFilter proves that the
// resume-items library-access gate drops entries from libraries the caller
// cannot access. The filter branch is only reached when allowedIDs is non-nil,
// which requires userAccess to be wired on the handler.
func TestPlaybackHandler_GetResumeItems_LibraryAccessFilter(t *testing.T) {
	h, progressRepo, mediaQueryRepo, libraryAccess := newTestPlaybackHandlerFull()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-res-ok", LibraryID: "lib-allowed", Type: "movie", Title: "Allowed"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-res-no", LibraryID: "lib-forbidden", Type: "movie", Title: "Forbidden"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-allowed")

	// GetResumeItems requires incomplete progress with a non-zero position.
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-res-ok", ProfileID: "profile-2", MediaItemID: "mi-res-ok",
		Position: mustPlaybackDurationMs(t, 30000), Completed: false, LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-res-no", ProfileID: "profile-2", MediaItemID: "mi-res-no",
		Position: mustPlaybackDurationMs(t, 30000), Completed: false, LastPlayedAt: time.Now(),
	})

	// Admin profile-1 resume items for the bypass sub-test.
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-res-adm-ok", ProfileID: "profile-1", MediaItemID: "mi-res-ok",
		Position: mustPlaybackDurationMs(t, 30000), Completed: false, LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-res-adm-no", ProfileID: "profile-1", MediaItemID: "mi-res-no",
		Position: mustPlaybackDurationMs(t, 30000), Completed: false, LastPlayedAt: time.Now(),
	})

	t.Run("restricted user sees only allowed-library items", func(t *testing.T) {
		resp, err := h.GetResumeItems(pbUserCtx("user-2", "profile-2"), &eveyv1.GetResumeItemsRequest{Limit: 10})
		require.NoError(t, err)
		ids := make([]string, len(resp.Progress))
		for i, p := range resp.Progress {
			ids[i] = p.MediaItemId
		}
		assert.Contains(t, ids, "mi-res-ok", "allowed-library resume item must appear")
		assert.NotContains(t, ids, "mi-res-no", "forbidden-library resume item must be filtered out")
	})

	t.Run("admin ctx bypasses library filter and sees both", func(t *testing.T) {
		resp, err := h.GetResumeItems(pbAdminCtx(), &eveyv1.GetResumeItemsRequest{Limit: 10})
		require.NoError(t, err)
		ids := make([]string, len(resp.Progress))
		for i, p := range resp.Progress {
			ids[i] = p.MediaItemId
		}
		assert.Contains(t, ids, "mi-res-ok", "admin must see the allowed-library resume item")
		assert.Contains(t, ids, "mi-res-no", "admin must see the forbidden-library resume item (no filter for admins)")
	})
}

// TestPlaybackHandler_GetNextUp_LibraryAccessFilter proves that the next-up
// library-access gate drops items from libraries the caller cannot access.
// The filter is only applied when allowedIDs is non-nil, which requires
// userAccess to be wired on the handler.
func TestPlaybackHandler_GetNextUp_LibraryAccessFilter(t *testing.T) {
	h, progressRepo, mediaQueryRepo, libraryAccess := newTestPlaybackHandlerFull()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-nup-ok", LibraryID: "lib-allowed", Type: "episode", Title: "Allowed"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-nup-no", LibraryID: "lib-forbidden", Type: "episode", Title: "Forbidden"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-allowed")

	// GetNextUp requires completed progress.
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-nup-ok", ProfileID: "profile-2", MediaItemID: "mi-nup-ok",
		Completed: true, LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-nup-no", ProfileID: "profile-2", MediaItemID: "mi-nup-no",
		Completed: true, LastPlayedAt: time.Now(),
	})

	// Admin profile-1 next-up items for the bypass sub-test.
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-nup-adm-ok", ProfileID: "profile-1", MediaItemID: "mi-nup-ok",
		Completed: true, LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-nup-adm-no", ProfileID: "profile-1", MediaItemID: "mi-nup-no",
		Completed: true, LastPlayedAt: time.Now(),
	})

	t.Run("restricted user sees only allowed-library items", func(t *testing.T) {
		resp, err := h.GetNextUp(pbUserCtx("user-2", "profile-2"), &eveyv1.GetNextUpRequest{Limit: 10})
		require.NoError(t, err)
		ids := make([]string, len(resp.Items))
		for i, it := range resp.Items {
			ids[i] = it.Id
		}
		assert.Contains(t, ids, "mi-nup-ok", "allowed-library next-up item must appear")
		assert.NotContains(t, ids, "mi-nup-no", "forbidden-library next-up item must be filtered out")
	})

	t.Run("admin ctx bypasses library filter and sees both", func(t *testing.T) {
		resp, err := h.GetNextUp(pbAdminCtx(), &eveyv1.GetNextUpRequest{Limit: 10})
		require.NoError(t, err)
		ids := make([]string, len(resp.Items))
		for i, it := range resp.Items {
			ids[i] = it.Id
		}
		assert.Contains(t, ids, "mi-nup-ok", "admin must see the allowed-library next-up item")
		assert.Contains(t, ids, "mi-nup-no", "admin must see the forbidden-library next-up item (no filter for admins)")
	})
}

// The audio bitrate row in the player's stream-info panel read "-" on a title whose
// audio we were re-encoding: a Matroska track rarely declares a bitrate, so the
// source probe has none - yet we are the ones choosing the figure.
//
// It must be the figure we actually hand the encoder, scaled to the channels being
// served. Quoting the unscaled base would tell a viewer 192 kbps while their 5.1
// track is encoded at 384 - a wrong number is worse than a dash.
func TestServedAudioBitrateBps(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }
	i := func(v int) *int { return &v }

	tests := []struct {
		name        string
		decision    transcoding.PlaybackDecision
		srcChannels *int
		srcBitrate  *int64
		want        int64
	}{
		{
			name:        "hybrid rung (video copied), 5.1: the rendition base scaled to 6 channels",
			decision:    transcoding.PlaybackDecision{AudioDecision: "transcode", VideoDecision: "copy", MaxAudioChannels: 6},
			srcChannels: i(6),
			want:        384_000, // 6 x 64k, the per-channel rule - not the stereo base
		},
		{
			// Derived from the constant: the panel must quote whatever the encoder
			// is handed, and that base moved once already (128 -> 192 on 2026-09-04,
			// see domain/transcoding/audio_bitrate.go). A literal here would let the
			// panel drift from the argv, which is the one thing this test exists for.
			name:        "hybrid rung, stereo: the rendition base, unscaled",
			decision:    transcoding.PlaybackDecision{AudioDecision: "transcode", VideoDecision: "copy", MaxAudioChannels: 6},
			srcChannels: i(2),
			want:        transcoding.AudioRenditionBaseKbps * 1000,
		},
		{
			name:        "full transcode, stereo: the ladder rung's own base",
			decision:    transcoding.PlaybackDecision{AudioDecision: "transcode", VideoDecision: "transcode", AudioBitrate: 192, MaxAudioChannels: 6},
			srcChannels: i(2),
			want:        192_000,
		},
		{
			name:        "7.1 source is served as 5.1: scaled to the CAP, not the source",
			decision:    transcoding.PlaybackDecision{AudioDecision: "transcode", VideoDecision: "copy", MaxAudioChannels: 6},
			srcChannels: i(8),
			want:        384_000,
		},
		{
			name:       "audio copied: the source's own rate",
			decision:   transcoding.PlaybackDecision{AudioDecision: "copy", VideoDecision: "copy"},
			srcBitrate: i64(640_000),
			want:       640_000,
		},
		{
			name:     "audio copied, source declares nothing: unknown, row hides",
			decision: transcoding.PlaybackDecision{AudioDecision: "copy", VideoDecision: "copy"},
			want:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			streams := []*library.MediaStream{{
				StreamType: library.Audio,
				Channels:   tc.srcChannels,
				Bitrate:    tc.srcBitrate,
			}}
			d := tc.decision
			assert.Equal(t, tc.want, servedAudioBitrateBps(&d, streams))
		})
	}
}

// During a cast the TV consumes admin commands itself (its alive beacon polls
// them); the phone's 3s progress tick must NOT win that race - it used to eat
// "pause" and apply it to its own muted local player, so the admin's button
// visibly did nothing on the TV (live repro 2026-07-29).
func TestPlaybackHandler_ReportProgress_CastSessionDoesNotConsumeCommand(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	startResp, err := h.StartStream(ctx, &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1",
		PlayMethod:  eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
		DeviceName:  "Phone",
	})
	require.NoError(t, err)
	require.NoError(t, h.cmd.SetRemoteCommand(ctx, startResp.Id, "pause"))

	// Phone tick WHILE CASTING (castDeviceName set): command left in place.
	resp, err := h.ReportProgress(ctx, &eveyv1.ReportProgressRequest{
		SessionId: startResp.Id, PositionMs: 1000, CastDeviceName: "Salon TV",
	})
	require.NoError(t, err)
	assert.Empty(t, resp.RemoteCommand, "a casting phone must not consume the TV's command")

	// The TV's poll gets it.
	cmd, err := h.cmd.ConsumeCastCommand(ctx, "profile-1", "mi-1")
	require.NoError(t, err)
	assert.Equal(t, "pause", cmd)

	// Local playback (no castDeviceName): the phone consumes as always.
	require.NoError(t, h.cmd.SetRemoteCommand(ctx, startResp.Id, "play"))
	resp, err = h.ReportProgress(ctx, &eveyv1.ReportProgressRequest{
		SessionId: startResp.Id, PositionMs: 2000,
	})
	require.NoError(t, err)
	assert.Equal(t, "play", resp.RemoteCommand)
}

// getAllowedLibraryIDs returns nil on error, and nil reads as "unrestricted"
// everywhere downstream - the function says so itself two lines above, where an
// unwired port deliberately returns an EMPTY slice instead so a wiring
// regression denies. The three call sites discarded the error
// (`allowedIDs, _ :=`), so a database outage took the nil branch and widened
// visibility instead of narrowing it. That is a fail-OPEN in a codebase whose
// stated posture everywhere else is fail-closed.
func TestPlaybackHandler_LibraryAccessLookupFailure_DeniesRatherThanOpens(t *testing.T) {
	h, progressRepo, mediaQueryRepo, libraryAccess := newTestPlaybackHandlerFull()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-fail-ok", LibraryID: "lib-allowed", Type: "movie", Title: "Allowed"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-fail-no", LibraryID: "lib-forbidden", Type: "movie", Title: "Forbidden"})

	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-allowed"))

	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-fail-ok", ProfileID: "profile-2", MediaItemID: "mi-fail-ok",
		Position: mustPlaybackDurationMs(t, 10000), LastPlayedAt: time.Now(),
	})
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-fail-no", ProfileID: "profile-2", MediaItemID: "mi-fail-no",
		Position: mustPlaybackDurationMs(t, 10000), LastPlayedAt: time.Now(),
	})

	// The grant store goes down AFTER the grants exist.
	libraryAccess.Err = errors.New("library_access unavailable")

	resp, err := h.GetWatchHistory(pbUserCtx("user-2", "profile-2"), &eveyv1.GetWatchHistoryRequest{Limit: 10})
	require.NoError(t, err)
	ids := make([]string, len(resp.Items))
	for i, it := range resp.Items {
		ids[i] = it.MediaItemId
	}
	assert.NotContains(t, ids, "mi-fail-no",
		"the grant lookup failed, so the caller's allowed set is UNKNOWN - showing a "+
			"forbidden-library item means a database outage widened what the user can see")
}

// The stream list is the whole input of the playback decision: it starts at
// direct_play/copy/copy and only the rules that ITERATE over the streams can
// demote it. Logging the read error and carrying on with a nil list therefore
// does not degrade the answer, it invents one - the server hands back a play
// method (direct play, or direct stream when only the container rule fires)
// without ever having looked at what is inside the file, and the client acts
// on it. An item with no streams recorded is NOT this case: that read
// succeeds, and answering optimistically for it stays deliberate.
func TestPlaybackHandler_GetStreamInfo_StreamReadErrorIsNotDirectPlay(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-1",
		Title: "Some Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mkv"},
	})
	streamRepo.Err = errors.New("db down")

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-1"})
	if err == nil {
		t.Fatalf("want an error when the streams cannot be read, got PlayMethod=%v", resp.PlayMethod)
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("want Internal, got %v", err)
	}
}

// The mark-watched pair shares one error contract: another profile's id is
// refused up front, a profile that vanished (deleted while the client still
// held its token) answers NotFound, and a storage failure is a sanitized
// Internal rather than a leaked message or a 404 the client would stop
// retrying on.
func TestPlaybackHandler_MarkWatched_ErrorMapping(t *testing.T) {
	h, _, progressRepo, _, profileRepo, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	_, err := h.MarkAsWatched(ctx, &eveyv1.MarkAsWatchedRequest{MediaItemId: "mi-1", ProfileId: "profile-other"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = h.MarkAsUnwatched(ctx, &eveyv1.MarkAsUnwatchedRequest{MediaItemId: "mi-1", ProfileId: "profile-other"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	progressRepo.Err = errors.New("disk full at /var/lib/evey")
	_, err = h.MarkAsWatched(ctx, &eveyv1.MarkAsWatchedRequest{MediaItemId: "mi-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.NotContains(t, err.Error(), "/var/lib")
	_, err = h.MarkAsUnwatched(ctx, &eveyv1.MarkAsUnwatchedRequest{MediaItemId: "mi-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.NotContains(t, err.Error(), "/var/lib")
	progressRepo.Err = nil

	require.NoError(t, profileRepo.Delete(context.Background(), "profile-1"))
	_, err = h.MarkAsWatched(ctx, &eveyv1.MarkAsWatchedRequest{MediaItemId: "mi-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = h.MarkAsUnwatched(ctx, &eveyv1.MarkAsUnwatchedRequest{MediaItemId: "mi-1"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	// A transient profile lookup failure is NOT a 404: the client must retry.
	profileRepo.Err = errors.New("db down")
	_, err = h.MarkAsWatched(ctx, &eveyv1.MarkAsWatchedRequest{MediaItemId: "mi-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

// A file stored in one-track blocks is never handed out as a raw direct-play
// URL: routed to the HLS master, where the VOD gate answers the typed 503
// (file_needs_remux) every client renders instead of a stall.
func TestGetStreamInfo_ChunkedFile_RoutesToHLSForTheRefusal(t *testing.T) {
	h, _, _, mediaQuery, _, streamRepo := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:    "mi-chunked",
		Title: "Blocks Movie",
		Type:  "movie",
		File:  &library.MediaFile{ContainerFormat: "mp4", InterleaveBackstepMs: 366_000},
	})
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{MediaItemID: "mi-chunked", StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{MediaItemID: "mi-chunked", StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	})

	resp, err := h.GetStreamInfo(ctx, &eveyv1.GetStreamInfoRequest{MediaItemId: "mi-chunked", SupportedAudioCodecs: []string{"aac"}})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE, resp.PlayMethod)
	assert.Contains(t, resp.TranscodeUrl, "hls/master.m3u8")
	assert.Empty(t, resp.DirectPlayUrl)
	// The DECISION carries the reason (the handler no longer rejudges the
	// file): the client can name why, and both tracks keep their copy verdicts.
	assert.Contains(t, resp.TranscodeReasons, "file_needs_remux")
	assert.Equal(t, "copy", resp.VideoDecision)
	assert.Equal(t, "copy", resp.AudioDecision)
}
