package notification_test

import (
	"context"
	"errors"
	"testing"

	appnotification "github.com/evey-project/evey/internal/app/notification"
	notiffakes "github.com/evey-project/evey/internal/domain/notification/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
