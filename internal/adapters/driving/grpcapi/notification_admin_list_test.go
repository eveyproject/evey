package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestListAdminNotifications_NonAdmin verifies the defensive IsAdmin check
// in the handler: even without the RBAC interceptor, a non-admin caller gets
// PermissionDenied.
func TestListAdminNotifications_NonAdmin(t *testing.T) {
	h, _, _ := newNotificationHandler()

	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.ListAdminNotifications(ctx, &eveyv1.ListAdminNotificationsRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// TestListAdminNotifications_Unauthenticated verifies that missing claims
// return an error (requireClaims path).
func TestListAdminNotifications_Unauthenticated(t *testing.T) {
	h, _, _ := newNotificationHandler()

	_, err := h.ListAdminNotifications(context.Background(), &eveyv1.ListAdminNotificationsRequest{})

	require.Error(t, err)
}

// TestListAdminNotifications_EmptyForAdmin verifies that an admin with no
// admin-only notifications receives an empty list without error.
func TestListAdminNotifications_EmptyForAdmin(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	// Seed a personal notification (not admin-only) for the admin profile.
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n-personal", ProfileID: "profile-admin", Type: notification.TypeNewMovie,
		Title: "New Movie", Message: "msg", CreatedAt: time.Now(),
	})

	ctx := ctxWithClaims("admin-user", "profile-admin", true)
	resp, err := h.ListAdminNotifications(ctx, &eveyv1.ListAdminNotificationsRequest{})

	require.NoError(t, err)
	assert.Empty(t, resp.Notifications)
}

// TestListAdminNotifications_ReturnsAdminOnlyNotifications verifies that
// admin-only notification types (plugin_error, backup_failed) are returned
// and personal types are excluded.
func TestListAdminNotifications_ReturnsAdminOnlyNotifications(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	now := time.Now()
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n-plugin", ProfileID: "profile-admin", Type: notification.TypePluginError,
		Title: "Plugin crashed", Message: "err", CreatedAt: now,
	})
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n-backup", ProfileID: "profile-admin", Type: notification.TypeBackupFailed,
		Title: "Backup failed", Message: "disk full", CreatedAt: now,
	})
	// Personal notification - must be filtered out.
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n-movie", ProfileID: "profile-admin", Type: notification.TypeNewMovie,
		Title: "New Movie", Message: "msg", CreatedAt: now,
	})

	ctx := ctxWithClaims("admin-user", "profile-admin", true)
	resp, err := h.ListAdminNotifications(ctx, &eveyv1.ListAdminNotificationsRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Notifications, 2)

	ids := []string{resp.Notifications[0].Id, resp.Notifications[1].Id}
	assert.Contains(t, ids, "n-plugin")
	assert.Contains(t, ids, "n-backup")
}

// TestListAdminNotifications_RepoError verifies that a repository error is
// mapped to an internal gRPC error.
func TestListAdminNotifications_RepoError(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	// Setting Err on the repo causes ListByProfile to return an error.
	repo.Err = assert.AnError

	ctx := ctxWithClaims("admin-user", "profile-admin", true)
	_, err := h.ListAdminNotifications(ctx, &eveyv1.ListAdminNotificationsRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// TestListAdminNotifications_UnreadOnlyFilter verifies that unreadOnly=true
// filters out already-read admin notifications.
func TestListAdminNotifications_UnreadOnlyFilter(t *testing.T) {
	h, repo, _ := newNotificationHandler()

	now := time.Now()
	readAt := now.Add(-time.Minute)
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n-read", ProfileID: "profile-admin", Type: notification.TypePluginError,
		Title: "Read plugin error", Message: "old", CreatedAt: now, ReadAt: &readAt,
	})
	_ = repo.Create(context.Background(), &notification.Notification{
		ID: "n-unread", ProfileID: "profile-admin", Type: notification.TypeBackupFailed,
		Title: "Unread backup fail", Message: "new", CreatedAt: now,
	})

	ctx := ctxWithClaims("admin-user", "profile-admin", true)
	resp, err := h.ListAdminNotifications(ctx, &eveyv1.ListAdminNotificationsRequest{UnreadOnly: true})

	require.NoError(t, err)
	require.Len(t, resp.Notifications, 1)
	assert.Equal(t, "n-unread", resp.Notifications[0].Id)
}
