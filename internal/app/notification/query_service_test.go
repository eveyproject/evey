package notification_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	appnotification "github.com/evey-project/evey/internal/app/notification"
	"github.com/evey-project/evey/internal/domain/notification"
	notiffakes "github.com/evey-project/evey/internal/domain/notification/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustCreate(t *testing.T, repo notification.Repository, profileID, id string, typ notification.Type, read bool) {
	t.Helper()
	n, err := notification.NewNotification(id, shared.ProfileID(profileID), typ, "title")
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), n))
	if read {
		require.NoError(t, repo.MarkRead(context.Background(), id, profileID))
	}
}

func TestQueryService_List_FiltersAdminOnlyTypes(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	svc := appnotification.NewQueryService(repo, settings)

	mustCreate(t, repo, "p1", "n1", notification.TypeNewMovie, false)
	mustCreate(t, repo, "p1", "n2", notification.TypePluginError, false)
	mustCreate(t, repo, "p1", "n3", notification.TypeNewEpisode, false)
	mustCreate(t, repo, "p1", "n4", notification.TypeBackupFailed, false)
	mustCreate(t, repo, "p1", "n5", notification.TypeNewSeries, false)

	got, err := svc.List(context.Background(), "p1", false, 50, 0)
	require.NoError(t, err)
	require.Len(t, got, 3, "admin-only types must not appear in user feed")
	for _, n := range got {
		assert.False(t, n.Type.IsAdminOnly(), "got admin-only type %q in user list", n.Type)
	}
}

func TestQueryService_UnreadCount_ExcludesAdminOnly(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	svc := appnotification.NewQueryService(repo, settings)

	mustCreate(t, repo, "p1", "n1", notification.TypeNewMovie, false)     // unread, personal
	mustCreate(t, repo, "p1", "n2", notification.TypePluginError, false)  // unread, admin-only
	mustCreate(t, repo, "p1", "n3", notification.TypeNewEpisode, true)    // read
	mustCreate(t, repo, "p1", "n4", notification.TypeBackupFailed, false) // unread, admin-only
	mustCreate(t, repo, "p1", "n5", notification.TypeNewSeries, false)    // unread, personal

	count, err := svc.UnreadCount(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, 2, count, "only n1 and n5 are unread+personal — admin-only excluded")
}

func TestQueryService_List_HonoursLimit(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	svc := appnotification.NewQueryService(repo, settings)

	for i := 0; i < 10; i++ {
		mustCreate(t, repo, "p1", "n"+strconv.Itoa(i), notification.TypeNewEpisode, false)
	}
	got, err := svc.List(context.Background(), "p1", false, 3, 0)
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

// ListPushSubscriptions returns empty (not nil) when WithPushSubs was not provided.
func TestQueryService_ListPushSubscriptions_NotConfigured_EmptySlice(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	qs := appnotification.NewQueryService(repo, settings)

	subs, err := qs.ListPushSubscriptions(context.Background(), "u1")
	require.NoError(t, err)
	assert.NotNil(t, subs)
	assert.Len(t, subs, 0)
}

// WithPushSubs wires the repo and lists entries per user.
func TestQueryService_ListPushSubscriptions_WithSubs_FiltersByUser(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	subsRepo := notiffakes.NewFakePushSubscriptionRepository()
	_ = subsRepo.Upsert(context.Background(), "u1", "p1", "https://push/u1-a", "k1", "a1")
	_ = subsRepo.Upsert(context.Background(), "u1", "p1", "https://push/u1-b", "k2", "a2")
	_ = subsRepo.Upsert(context.Background(), "u2", "p2", "https://push/u2-a", "k3", "a3")

	qs := appnotification.NewQueryService(repo, settings, appnotification.WithPushSubs(subsRepo))

	subs, err := qs.ListPushSubscriptions(context.Background(), "u1")
	require.NoError(t, err)
	assert.Len(t, subs, 2)

	other, err := qs.ListPushSubscriptions(context.Background(), "u2")
	require.NoError(t, err)
	assert.Len(t, other, 1)

	none, err := qs.ListPushSubscriptions(context.Background(), "u-missing")
	require.NoError(t, err)
	assert.Len(t, none, 0)
}

// ListPushSubscriptions propagates the repo error when WithPushSubs is wired.
func TestQueryService_ListPushSubscriptions_RepoError_Propagates(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	subsRepo := notiffakes.NewFakePushSubscriptionRepository()
	subsRepo.Err = errors.New("db down")

	qs := appnotification.NewQueryService(repo, settings, appnotification.WithPushSubs(subsRepo))

	_, err := qs.ListPushSubscriptions(context.Background(), "u1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
