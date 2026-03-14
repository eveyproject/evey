package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	"github.com/gravity-zero/evey/internal/domain/notification"
	notiffakes "github.com/gravity-zero/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newNotificationHandler() (*notificationHandler, *notiffakes.FakeNotificationRepository, *notiffakes.FakeSettingsRepository) {
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	analyzer := notiffakes.NewFakeWatchHistoryAnalyzer()
	profiles := userfakes.NewFakeProfileRepository()
	events := sharedfakes.NewFakeEventBus()

	cmd := appnotification.NewCommandService(repo, settings, analyzer, profiles, events)
	query := appnotification.NewQueryService(repo, settings)
	h := &notificationHandler{cmd: cmd, query: query}
	return h, repo, settings
}

func TestNotificationHandler_ListNotifications_Success(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	now := time.Now()
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n1", ProfileID: "profile-1", Type: notification.TypeNewMovie,
		Title: "New Movie", Message: "Movie added", CreatedAt: now,
	})
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n2", ProfileID: "profile-1", Type: notification.TypeNewEpisode,
		Title: "New Episode", Message: "Episode added", CreatedAt: now,
	})
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n3", ProfileID: "profile-2", Type: notification.TypeNewMovie,
		Title: "Other", Message: "Other profile", CreatedAt: now,
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ListNotifications(ctx, &eveyv1.ListNotificationsRequest{Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Notifications, 2)
	assert.Equal(t, "n1", resp.Notifications[0].Id)
	assert.Equal(t, "n2", resp.Notifications[1].Id)
}

func TestNotificationHandler_ListNotifications_Unauthenticated(t *testing.T) {
	h, _, _ := newNotificationHandler()

	_, err := h.ListNotifications(context.Background(), &eveyv1.ListNotificationsRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestNotificationHandler_MarkRead_Success(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n1", ProfileID: "profile-1", Type: notification.TypeNewMovie,
		Title: "Test", Message: "Test", CreatedAt: time.Now(),
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.MarkRead(ctx, &eveyv1.MarkReadRequest{Id: "n1"})

	require.NoError(t, err)
	assert.NotNil(t, resp)

	count, _ := repo.UnreadCount(context.Background(), "profile-1")
	assert.Equal(t, 0, count)
}

func TestNotificationHandler_MarkAllRead_Success(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	now := time.Now()
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n1", ProfileID: "profile-1", Type: notification.TypeNewMovie,
		Title: "M1", Message: "msg", CreatedAt: now,
	})
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n2", ProfileID: "profile-1", Type: notification.TypeNewEpisode,
		Title: "M2", Message: "msg", CreatedAt: now,
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.MarkAllRead(ctx, &eveyv1.MarkAllReadRequest{})

	require.NoError(t, err)
	assert.NotNil(t, resp)

	count, _ := repo.UnreadCount(context.Background(), "profile-1")
	assert.Equal(t, 0, count)
}

func TestNotificationHandler_GetUnreadCount_Success(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	now := time.Now()
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n1", ProfileID: "profile-1", Type: notification.TypeNewMovie,
		Title: "M1", Message: "msg", CreatedAt: now,
	})
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n2", ProfileID: "profile-1", Type: notification.TypeNewEpisode,
		Title: "M2", Message: "msg", CreatedAt: now,
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetUnreadCount(ctx, &eveyv1.GetUnreadCountRequest{})

	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Count)
}

func TestNotificationHandler_DeleteAllNotifications_Success(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n1", ProfileID: "profile-1", Type: notification.TypeNewMovie,
		Title: "M1", Message: "msg", CreatedAt: time.Now(),
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.DeleteAllNotifications(ctx, &eveyv1.DeleteAllNotificationsRequest{})

	require.NoError(t, err)
	assert.NotNil(t, resp)

	items, _ := repo.ListByProfile(context.Background(), "profile-1", false, 100, 0)
	assert.Empty(t, items)
}

func TestNotificationHandler_GetNotificationSettings_Success(t *testing.T) {
	h, _, _ := newNotificationHandler()

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetNotificationSettings(ctx, &eveyv1.GetNotificationSettingsRequest{})

	require.NoError(t, err)
	assert.Equal(t, "profile-1", resp.ProfileId)
	assert.Equal(t, eveyv1.NotificationLevel_NOTIFICATION_LEVEL_ALL, resp.MovieLevel)
}

func TestNotificationHandler_UpdateNotificationSettings_Success(t *testing.T) {
	h, _, _ := newNotificationHandler()

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.UpdateNotificationSettings(ctx, &eveyv1.UpdateNotificationSettingsRequest{
		MovieLevel:  eveyv1.NotificationLevel_NOTIFICATION_LEVEL_DISABLED,
		SeriesLevel: eveyv1.NotificationLevel_NOTIFICATION_LEVEL_RELEVANT,
		MusicLevel:  eveyv1.NotificationLevel_NOTIFICATION_LEVEL_ALL,
	})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.NotificationLevel_NOTIFICATION_LEVEL_DISABLED, resp.MovieLevel)
	assert.Equal(t, eveyv1.NotificationLevel_NOTIFICATION_LEVEL_RELEVANT, resp.SeriesLevel)
}

func TestNotificationHandler_GetGlobalNotificationSettings_Success(t *testing.T) {
	h, _, _ := newNotificationHandler()

	resp, err := h.GetGlobalNotificationSettings(context.Background(), &eveyv1.GetGlobalNotificationSettingsRequest{})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.NotificationLevel_NOTIFICATION_LEVEL_ALL, resp.MovieLevel)
}

func TestNotificationHandler_UpdateGlobalNotificationSettings_Success(t *testing.T) {
	h, _, _ := newNotificationHandler()

	resp, err := h.UpdateGlobalNotificationSettings(context.Background(), &eveyv1.UpdateGlobalNotificationSettingsRequest{
		MovieLevel:  eveyv1.NotificationLevel_NOTIFICATION_LEVEL_HIGHLY_RELEVANT,
		SeriesLevel: eveyv1.NotificationLevel_NOTIFICATION_LEVEL_DISABLED,
		MusicLevel:  eveyv1.NotificationLevel_NOTIFICATION_LEVEL_ALL,
	})

	require.NoError(t, err)
	assert.Equal(t, eveyv1.NotificationLevel_NOTIFICATION_LEVEL_HIGHLY_RELEVANT, resp.MovieLevel)
	assert.Equal(t, eveyv1.NotificationLevel_NOTIFICATION_LEVEL_DISABLED, resp.SeriesLevel)
}
