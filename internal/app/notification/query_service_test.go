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
		require.NoError(t, repo.MarkRead(context.Background(), id, shared.ProfileID(profileID)))
	}
}

// Admin-only rows are only ever CREATED on admin profiles (the event handlers
// target them via ListAdminProfiles), so hiding them from the owner's feed
// hid them from everyone: in prod (2026-08-26) media requests piled up for
// weeks without the operator ever being paged - no badge, no dropdown entry,
// and no client calls the dedicated ListAdminNotifications endpoint. The feed
// must therefore INCLUDE them; ListAdminOnly stays as a filtered view.
func TestQueryService_List_IncludesAdminOnlyTypes(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	svc := appnotification.NewQueryService(repo, settings)

	mustCreate(t, repo, "p1", "n1", notification.TypeNewMovie, false)
	mustCreate(t, repo, "p1", "n2", notification.TypePluginError, false)
	mustCreate(t, repo, "p1", "n3", notification.TypeNewEpisode, false)
	mustCreate(t, repo, "p1", "n4", notification.TypeMediaRequest, false)
	mustCreate(t, repo, "p1", "n5", notification.TypeNewSeries, false)

	got, err := svc.List(context.Background(), "p1", false, 50, 0)
	require.NoError(t, err)
	require.Len(t, got, 5, "the owner's feed must carry every row written for this profile, admin-only included")
	types := make(map[notification.Type]bool, len(got))
	for _, n := range got {
		types[n.Type] = true
	}
	assert.True(t, types[notification.TypeMediaRequest], "a media request must page the admin in their own feed")
	assert.True(t, types[notification.TypePluginError])
}

func TestQueryService_UnreadCount_CountsAdminOnly(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	svc := appnotification.NewQueryService(repo, settings)

	mustCreate(t, repo, "p1", "n1", notification.TypeNewMovie, false)     // unread, personal
	mustCreate(t, repo, "p1", "n2", notification.TypeMediaRequest, false) // unread, admin-only
	mustCreate(t, repo, "p1", "n3", notification.TypeNewEpisode, true)    // read
	mustCreate(t, repo, "p1", "n4", notification.TypeBackupFailed, false) // unread, admin-only
	mustCreate(t, repo, "p1", "n5", notification.TypeNewSeries, false)    // unread, personal

	count, err := svc.UnreadCount(context.Background(), "p1")
	require.NoError(t, err)
	assert.Equal(t, 4, count, "the badge must count admin-only rows - they are what pages the operator")
}

// ListAdminOnly is the inverse of List: it surfaces plugin_error and
// backup_failed for an admin's profile, dropping personal types so the
// admin-side bell isn't noised by new_movie / new_episode / new_series.
func TestQueryService_ListAdminOnly_KeepsOnlyAdminTypes(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	svc := appnotification.NewQueryService(repo, settings)

	mustCreate(t, repo, "p1", "n1", notification.TypeNewMovie, false)
	mustCreate(t, repo, "p1", "n2", notification.TypePluginError, false)
	mustCreate(t, repo, "p1", "n3", notification.TypeNewEpisode, false)
	mustCreate(t, repo, "p1", "n4", notification.TypeBackupFailed, false)
	mustCreate(t, repo, "p1", "n5", notification.TypeNewSeries, false)

	got, err := svc.ListAdminOnly(context.Background(), "p1", false, 50, 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	ids := []string{got[0].ID, got[1].ID}
	assert.Contains(t, ids, "n2")
	assert.Contains(t, ids, "n4")
	for _, n := range got {
		assert.True(t, n.Type.IsAdminOnly(), "personal type leaked into admin feed: %q", n.Type)
	}
}

// ListAdminOnly with unreadOnly=true filters by read state too - relied on
// by the admin badge counter.
func TestQueryService_ListAdminOnly_UnreadOnly(t *testing.T) {
	t.Parallel()
	repo := notiffakes.NewFakeNotificationRepository()
	settings := notiffakes.NewFakeSettingsRepository()
	svc := appnotification.NewQueryService(repo, settings)

	mustCreate(t, repo, "p1", "n1", notification.TypePluginError, false)
	mustCreate(t, repo, "p1", "n2", notification.TypePluginError, true)
	mustCreate(t, repo, "p1", "n3", notification.TypeBackupFailed, false)

	got, err := svc.ListAdminOnly(context.Background(), "p1", true, 50, 0)
	require.NoError(t, err)
	assert.Len(t, got, 2, "n2 is read, must be excluded when unreadOnly=true")
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
