package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/notification"
	notiffakes "github.com/evey-project/evey/internal/domain/notification/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
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

// levelFromProto maps a client-controlled enum straight into a domain value
// used to filter what notifications a profile receives; a swapped case would
// silently mute or over-notify a user without any handler test catching it
// (existing handler tests only exercise DISABLED/ALL/RELEVANT/HIGHLY_RELEVANT
// round-trips, never the unspecified/unknown fallback).
func TestLevelFromProto(t *testing.T) {
	cases := []struct {
		name string
		in   eveyv1.NotificationLevel
		want notification.Level
	}{
		{"disabled", eveyv1.NotificationLevel_NOTIFICATION_LEVEL_DISABLED, notification.LevelDisabled},
		{"all", eveyv1.NotificationLevel_NOTIFICATION_LEVEL_ALL, notification.LevelAll},
		{"relevant", eveyv1.NotificationLevel_NOTIFICATION_LEVEL_RELEVANT, notification.LevelRelevant},
		{"highly relevant", eveyv1.NotificationLevel_NOTIFICATION_LEVEL_HIGHLY_RELEVANT, notification.LevelHighlyRelevant},
		// Unspecified (zero value on the wire, e.g. field omitted by an older
		// client) and an unknown enum value from a newer client must both fall
		// back to the same default as the rest of the domain (notification.Level
		// default is LevelAll), not silently disable notifications.
		{"unspecified falls back to all", eveyv1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED, notification.LevelAll},
		{"unknown future enum falls back to all", eveyv1.NotificationLevel(9999), notification.LevelAll},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, levelFromProto(tc.in))
		})
	}
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

// TestNotificationHandler_MarkRead_IDORAnotherProfile proves the per-profile
// scoping on MarkRead: a different profile marking this profile's notification
// is a silent no-op (the command scopes the write by claims.ProfileID), so the
// owner's notification stays unread. Mirrors the existing List/GetUnreadCount
// IDOR tests, which MarkRead lacked.
func TestNotificationHandler_MarkRead_IDORAnotherProfile(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n1", ProfileID: "profile-1", Type: notification.TypeNewMovie,
		Title: "Owned by profile-1", Message: "msg", CreatedAt: time.Now(),
	})

	// profile-2 attempts to mark profile-1's notification read.
	ctx := ctxWithClaims("user-2", "profile-2", false)
	_, err := h.MarkRead(ctx, &eveyv1.MarkReadRequest{Id: "n1"})
	require.NoError(t, err) // silent no-op (0 rows match), not an error

	// profile-1's notification must remain unread.
	count, _ := repo.UnreadCount(context.Background(), "profile-1")
	assert.Equal(t, 1, count, "another profile must not mark this profile's notification read")
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

// TestNotificationHandler_ListNotifications_NegativeOffsetClamped proves that
// req.Offset=-1 is clamped to 0 before reaching the repository. The fake repo
// does out[-1:] for negative offset on non-empty results, which panics - a
// passing test therefore demonstrates the clamp is in place.
func TestNotificationHandler_ListNotifications_NegativeOffsetClamped(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	// Seed at least one notification so the repo slice is non-empty.
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n1", ProfileID: "profile-1", Type: notification.TypeNewMovie,
		Title: "Movie", Message: "msg", CreatedAt: time.Now(),
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	// Must not panic; the handler must clamp -1 to 0.
	resp, err := h.ListNotifications(ctx, &eveyv1.ListNotificationsRequest{Offset: -1})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// IDOR regression guard - non-admin must not list or mutate another profile's notifications.
func TestNotificationHandler_ListNotifications_IDORAnotherProfile(t *testing.T) {
	h, _, _ := newNotificationHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.ListNotifications(ctx, &eveyv1.ListNotificationsRequest{ProfileId: "profile-stranger"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestNotificationHandler_GetUnreadCount_IDORAnotherProfile(t *testing.T) {
	h, _, _ := newNotificationHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.GetUnreadCount(ctx, &eveyv1.GetUnreadCountRequest{ProfileId: "profile-stranger"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// Library scoping of the bell: a restricted member only sees notifications
// about titles in libraries they were granted (a deleted title's row is
// dropped too), an admin sees everything, an unwired access port fails
// closed rather than reading as "unrestricted", and a lookup failure is an
// error, never a leak.
func TestNotificationHandler_ListNotifications_LibraryScoped(t *testing.T) {
	h, repo, _ := newNotificationHandler()
	ctx := context.Background()
	now := time.Now()
	for _, n := range []*notification.Notification{
		{ID: "n-mine", ProfileID: "profile-1", Type: notification.TypeNewMovie, Title: "Mine", MediaItemID: "mi-mine", CreatedAt: now},
		{ID: "n-other", ProfileID: "profile-1", Type: notification.TypeNewMovie, Title: "Other lib", MediaItemID: "mi-other", CreatedAt: now.Add(-time.Second)},
		{ID: "n-gone", ProfileID: "profile-1", Type: notification.TypeNewMovie, Title: "Deleted", MediaItemID: "mi-gone", CreatedAt: now.Add(-2 * time.Second)},
		{ID: "n-system", ProfileID: "profile-1", Type: notification.TypeBackupFailed, Title: "Backup", CreatedAt: now.Add(-3 * time.Second)},
	} {
		require.NoError(t, repo.Create(ctx, n))
	}
	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	mediaRepo.Seed(&library.MediaItem{ID: "mi-mine", LibraryID: "lib-mine", Title: "Mine"})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-other", LibraryID: "lib-other", Title: "Other"})
	h.libQuery = applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	h.userAccess = appuser.NewAccessService(userfakes.NewFakeUserRepository(), userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(), appuser.WithLibraryAccess(libAccess))
	ids := func(resp *eveyv1.ListNotificationsResponse) []string {
		out := make([]string, 0, len(resp.Notifications))
		for _, n := range resp.Notifications {
			out = append(out, n.Id)
		}
		return out
	}

	// Restricted member: granted lib-mine only.
	require.NoError(t, libAccess.Grant(ctx, "user-1", "lib-mine"))
	resp, err := h.ListNotifications(ctxWithClaims("user-1", "profile-1", false), &eveyv1.ListNotificationsRequest{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"n-mine", "n-system"}, ids(resp), "other library and deleted title dropped, system notice kept")

	// Admin: no filtering at all, the deleted title's row included.
	resp, err = h.ListNotifications(ctxWithClaims("user-1", "profile-1", true), &eveyv1.ListNotificationsRequest{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Notifications, 4)

	// Unrestricted member (no grant rows): everything, like an admin.
	resp, err = h.ListNotifications(ctxWithClaims("user-free", "profile-1", false), &eveyv1.ListNotificationsRequest{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Notifications, 4)

	// Access lookup failing: fail closed with an error, never a full list.
	libAccess.Err = errors.New("db down")
	_, err = h.ListNotifications(ctxWithClaims("user-1", "profile-1", false), &eveyv1.ListNotificationsRequest{Limit: 10})
	assert.Equal(t, codes.Internal, status.Code(err))
	libAccess.Err = nil

	// Unwired access port: a member sees only title-less notices.
	h.userAccess = nil
	resp, err = h.ListNotifications(ctxWithClaims("user-1", "profile-1", false), &eveyv1.ListNotificationsRequest{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{"n-system"}, ids(resp))
}
